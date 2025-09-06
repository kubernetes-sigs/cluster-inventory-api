package secretreader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	clientauthv1beta1 "k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	v1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentialplugin"
)

type Provider struct {
    // Client, if set, is used instead of creating a dynamic client from rest.Config.
    Client dynamic.Interface
    // Namespace, if set, overrides namespace inference.
    Namespace string
}

// NewDefault constructs a Provider with a pre-initialized dynamic client and inferred namespace.
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

    dyn, err := dynamic.NewForConfig(cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to create dynamic client: %w", err)
    }

    return &Provider{Client: dyn, Namespace: inferNamespace()}, nil
}

const ProviderName = "secretreader"

// SecretTokenKey is the `Secret.data` key.
const SecretTokenKey = "token"

func (Provider) Name() string { return ProviderName }

func (p Provider) GetTokenJSON(ctx context.Context, info *clientauthv1beta1.ExecCredential) ([]byte, error) {
    // Require pre-initialized dynamic client
    if p.Client == nil {
        return nil, errors.New("provider client is not initialized; construct with NewDefault or set Client")
    }

    // Determine namespace: prefer injected Namespace, then kubeconfig current-context (fallback to "default").
    namespace := p.Namespace
    if namespace == "" {
        namespace = inferNamespace()
    }

    // Normalize incoming server for matching
    normIn, err := normalizeHost(info.Spec.Cluster.Server)
    if err != nil {
        return nil, err
    }

    // Discover ClusterProfile that matches server only
    dyn := p.Client
    // Register ClusterProfile scheme for typed conversion
    if err := v1alpha1.AddToScheme(k8sscheme.Scheme); err != nil {
        return nil, fmt.Errorf("failed to register ClusterProfile scheme: %w", err)
    }
    gvr := schema.GroupVersionResource{Group: "multicluster.x-k8s.io", Version: "v1alpha1", Resource: "clusterprofiles"}
    list, err := dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list ClusterProfiles: %w", err)
    }

    var typed v1alpha1.ClusterProfileList
    for i := range list.Items {
        u := &list.Items[i]
        var cp v1alpha1.ClusterProfile
        if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &cp); err != nil {
            continue
        }
        typed.Items = append(typed.Items, cp)
    }
    clusterName := pickClusterProfileName(&typed, ProviderName, normIn, info.Spec.Cluster.CertificateAuthorityData)
    if clusterName == "" {
        return nil, fmt.Errorf("no matching ClusterProfile for endpoint: %s", info.Spec.Cluster.Server)
    }

    // Read Secret <namespace>/<clusterName> and return token using dynamic client
    secU, err := dyn.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}).Namespace(namespace).Get(ctx, clusterName, metav1.GetOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, clusterName, err)
    }
    var sec corev1.Secret
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(secU.Object, &sec); err != nil {
        return nil, fmt.Errorf("failed to convert secret %s/%s: %w", namespace, clusterName, err)
    }
    data, ok := sec.Data[SecretTokenKey]
    if !ok || len(data) == 0 {
        return nil, fmt.Errorf("secret %s/%s missing %q key", namespace, clusterName, SecretTokenKey)
    }

    return credentialplugin.BuildExecCredentialJSON(string(data), metav1.Time{}.Time)
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


