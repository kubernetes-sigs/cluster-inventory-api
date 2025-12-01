package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentialplugin"
)

type Provider struct {
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

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &Provider{KubeClient: kubeClient, Namespace: inferNamespace()}, nil
}

// ProviderName is the name of the credential provider.
const ProviderName = "secretreader"

// SecretTokenKey is the `Secret.data` key.
const SecretTokenKey = "token"

func (Provider) Name() string { return ProviderName }

func (p Provider) GetToken(
	ctx context.Context,
	info clientauthenticationv1.ExecCredential,
) (clientauthenticationv1.ExecCredentialStatus, error) {
	// Require pre-initialized typed clients
	if p.KubeClient == nil {
		return clientauthenticationv1.ExecCredentialStatus{},
			errors.New("provider clients are not initialized; construct with NewDefault or set clients")
	}

	// Require clusterName to be present in extensions config
	type execClusterConfig struct {
		ClusterName string `json:"clusterName"`
	}
	// Validate presence of cluster config
	if info.Spec.Cluster == nil || len(info.Spec.Cluster.Config.Raw) == 0 {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("missing ExecCredential.Spec.Cluster.Config")
	}
	var cfg execClusterConfig
	if err := json.Unmarshal(info.Spec.Cluster.Config.Raw, &cfg); err != nil {
		return clientauthenticationv1.ExecCredentialStatus{},
			fmt.Errorf("invalid ExecCredential.Spec.Cluster.Config: %w", err)
	}
	if cfg.ClusterName == "" {
		return clientauthenticationv1.ExecCredentialStatus{},
			fmt.Errorf("missing clusterName in ExecCredential.Spec.Cluster.Config")
	}
	clusterName := cfg.ClusterName

	// Read Secret <namespace>/<clusterName> via typed client and return token
	sec, err := p.KubeClient.CoreV1().Secrets(p.Namespace).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{},
			fmt.Errorf("failed to get secret %s/%s: %w", p.Namespace, clusterName, err)
	}
	data, ok := sec.Data[SecretTokenKey]
	if !ok || len(data) == 0 {
		return clientauthenticationv1.ExecCredentialStatus{},
			fmt.Errorf("secret %s/%s missing %q key", p.Namespace, clusterName, SecretTokenKey)
	}

	return clientauthenticationv1.ExecCredentialStatus{Token: string(data)}, nil
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

func main() {
	p, err := NewDefault()
	if err != nil {
		panic(err)
	}
	credentialplugin.Run(*p)
}
