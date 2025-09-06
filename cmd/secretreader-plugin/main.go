package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	v1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	clusterinventoryapisclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentialplugin"
)

type Provider struct {
	// ClusterInventoryAPIClient is the typed client for ClusterProfile API group.
	ClusterInventoryAPIClient clusterinventoryapisclient.Interface
	// KubeClient is the typed client for core Kubernetes resources (e.g. Secret).
	KubeClient kubernetes.Interface
	// Namespace, if set, overrides namespace inference.
	Namespace string
}

// NewDefault constructs a Provider with pre-initialized typed clientsets and an inferred namespace.
func NewDefault() (*Provider, error) {
	// Build Kubernetes rest.Config via in-cluster first, then fallback to kubeconfig
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build kube client config: %w", err)
		}
	}

	apiClient, err := clusterinventoryapisclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create apis clientset: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &Provider{ClusterInventoryAPIClient: apiClient, KubeClient: kubeClient, Namespace: inferNamespace()}, nil
}

// ProviderName is the name of the credential provider.
const ProviderName = "secretreader"

// SecretTokenKey is the `Secret.data` key.
const SecretTokenKey = "token"

func (Provider) Name() string { return ProviderName }

func (p Provider) GetToken(ctx context.Context, info clientauthenticationv1.ExecCredential) (clientauthenticationv1.ExecCredentialStatus, error) {
	// Require pre-initialized typed clients
	if p.ClusterInventoryAPIClient == nil || p.KubeClient == nil {
		return clientauthenticationv1.ExecCredentialStatus{}, errors.New("provider clients are not initialized; construct with NewDefault or set clients")
	}

	// Determine namespace: prefer injected Namespace, then kubeconfig current-context (fallback to "default").
	namespace := p.Namespace
	if namespace == "" {
		namespace = inferNamespace()
	}

	// Normalize incoming server for matching
	normIn, err := normalizeHost(info.Spec.Cluster.Server)
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, err
	}

	// Discover ClusterProfile that matches server only (all namespaces)
	cps, err := p.ClusterInventoryAPIClient.ApisV1alpha1().ClusterProfiles(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("failed to list ClusterProfiles: %w", err)
	}
	clusterName := pickClusterProfileName(cps, ProviderName, normIn, info.Spec.Cluster.CertificateAuthorityData)
	if clusterName == "" {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("no matching ClusterProfile for endpoint: %s", info.Spec.Cluster.Server)
	}

	// Read Secret <namespace>/<clusterName> via typed client and return token
	sec, err := p.KubeClient.CoreV1().Secrets(namespace).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("failed to get secret %s/%s: %w", namespace, clusterName, err)
	}
	data, ok := sec.Data[SecretTokenKey]
	if !ok || len(data) == 0 {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("secret %s/%s missing %q key", namespace, clusterName, SecretTokenKey)
	}

	return clientauthenticationv1.ExecCredentialStatus{Token: string(data)}, nil
}

// normalizeHost converts a URL like https://example.com:443/ to example.com
func normalizeHost(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("empty host")
	}
	// Strip scheme if present
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err == nil {
			raw = u.Host + u.Path
		}
	}
	// Drop trailing slash
	raw = strings.TrimSuffix(raw, "/")
	// Drop :443 suffix if present
	raw = strings.TrimSuffix(raw, ":443")
	return raw, nil
}

// inferNamespace determines the namespace to read Secrets from, preferring kubeconfig current-context
func inferNamespace() string {
	// kubeconfig current-context namespace
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path := os.Getenv("KUBECONFIG"); strings.TrimSpace(path) != "" {
		rules.ExplicitPath = path
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	if n, _, err := cc.Namespace(); err == nil && strings.TrimSpace(n) != "" {
		return n
	}
	// in-cluster kubeconfig is unavailable; library returns default namespace
	return "default"
}

// pickClusterProfileName returns the first ClusterProfile name
// whose provider name matches and whose cluster {server, CA} matches the input.
//   - server: matched by normalizeHost equality
//   - CA: if inputCA is provided (len>0), require byte-equal with provider.Cluster.CertificateAuthorityData
//     if inputCA is empty, skip CA check
func pickClusterProfileName(list *v1alpha1.ClusterProfileList, providerName string, normalizedServer string, inputCA []byte) string {
	providerName = strings.TrimSpace(providerName)
	if list == nil || providerName == "" || normalizedServer == "" {
		return ""
	}
	for i := range list.Items {
		cp := &list.Items[i]
		for _, pr := range cp.Status.CredentialProviders {
			if strings.TrimSpace(pr.Name) != providerName {
				continue
			}
			normCp, err := normalizeHost(strings.TrimSpace(pr.Cluster.Server))
			if err != nil || normCp != normalizedServer {
				continue
			}
			if len(inputCA) > 0 {
				if !bytes.Equal(pr.Cluster.CertificateAuthorityData, inputCA) {
					continue
				}
			}
			return cp.GetName()
		}
	}
	return ""
}

func main() {
	p, err := NewDefault()
	if err != nil {
		panic(err)
	}
	credentialplugin.Run(*p)
}
