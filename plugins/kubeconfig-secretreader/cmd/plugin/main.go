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
const ProviderName = "kubeconfig-secretreader"

type execClusterConfig struct {
	Name      string `json:"name"`      // Secret name (required)
	Key       string `json:"key"`       // Secret.data key (required)
	Namespace string `json:"namespace"` // Optional: namespace to read Secret from
	Context   string `json:"context"`   // Optional: kubeconfig context name
}

func (Provider) Name() string { return ProviderName }

func (p Provider) GetToken(
	ctx context.Context,
	info clientauthenticationv1.ExecCredential,
) (clientauthenticationv1.ExecCredentialStatus, error) {
	// Require pre-initialized typed clients
	if p.KubeClient == nil {
		return clientauthenticationv1.ExecCredentialStatus{}, errors.New(
			"provider clients are not initialized; construct with NewDefault or set clients",
		)
	}

	// Validate presence of cluster config
	if info.Spec.Cluster == nil || len(info.Spec.Cluster.Config.Raw) == 0 {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("missing ExecCredential.Spec.Cluster.Config")
	}
	var cfg execClusterConfig
	if err := json.Unmarshal(info.Spec.Cluster.Config.Raw, &cfg); err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
			"invalid ExecCredential.Spec.Cluster.Config: %w",
			err,
		)
	}
	if cfg.Name == "" {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("missing name in ExecCredential.Spec.Cluster.Config")
	}
	if cfg.Key == "" {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("missing key in ExecCredential.Spec.Cluster.Config")
	}

	// Determine namespace: use provided namespace, or fallback to inferred namespace
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = p.Namespace
	}

	// Read Secret
	sec, err := p.KubeClient.CoreV1().Secrets(namespace).Get(ctx, cfg.Name, metav1.GetOptions{})
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
			"failed to get secret %s/%s: %w",
			namespace,
			cfg.Name,
			err,
		)
	}
	kubeconfigData, ok := sec.Data[cfg.Key]
	if !ok || len(kubeconfigData) == 0 {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
			"secret %s/%s missing %q key",
			namespace,
			cfg.Name,
			cfg.Key,
		)
	}

	// Parse kubeconfig
	config, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
			"failed to parse kubeconfig from secret %s/%s: %w",
			namespace,
			cfg.Name,
			err,
		)
	}

	// Check for unsupported extensions
	if len(config.Extensions) > 0 {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("kubeconfig extensions are not supported")
	}

	// Determine context: use provided context, or fallback to current-context
	contextName := cfg.Context
	if contextName == "" {
		contextName = config.CurrentContext
	}
	if contextName == "" {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
			"no context specified and no current-context in kubeconfig",
		)
	}

	// Get context
	context, ok := config.Contexts[contextName]
	if !ok {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}

	// Get user
	user, ok := config.AuthInfos[context.AuthInfo]
	if !ok {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("user %q not found in kubeconfig", context.AuthInfo)
	}

	// Build ExecCredentialStatus - support both token and client certificate/key
	status := clientauthenticationv1.ExecCredentialStatus{}

	// Handle token authentication
	if user.Token != "" {
		status.Token = user.Token
	}

	// Handle client certificate/key authentication
	hasClientCert := len(user.ClientCertificateData) > 0 || user.ClientCertificate != ""
	hasClientKey := len(user.ClientKeyData) > 0 || user.ClientKey != ""

	if hasClientCert || hasClientKey {
		// Both certificate and key must be present
		if !hasClientCert {
			return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
				"client-key-data found but no client-certificate-data in user %q",
				context.AuthInfo,
			)
		}
		if !hasClientKey {
			return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
				"client-certificate-data found but no client-key-data in user %q",
				context.AuthInfo,
			)
		}

		// Handle client-certificate-data
		if len(user.ClientCertificateData) > 0 {
			// Already decoded (PEM string)
			status.ClientCertificateData = string(user.ClientCertificateData)
		} else if user.ClientCertificate != "" {
			// File path - not supported in this plugin
			return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
				"client-certificate file path is not supported; use client-certificate-data",
			)
		}

		// Handle client-key-data
		if len(user.ClientKeyData) > 0 {
			// Already decoded (PEM string)
			status.ClientKeyData = string(user.ClientKeyData)
		} else if user.ClientKey != "" {
			// File path - not supported in this plugin
			return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
				"client-key file path is not supported; use client-key-data",
			)
		}
	}

	// At least one authentication method must be present
	if status.Token == "" && status.ClientCertificateData == "" {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf(
			"no authentication method found in user %q "+
				"(neither token nor client-certificate-data/client-key-data)",
			context.AuthInfo,
		)
	}

	return status, nil
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
	// in-cluster: try reading from service account namespace file
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}
	// fallback to default namespace
	return "default"
}

func main() {
	p, err := NewDefault()
	if err != nil {
		panic(err)
	}
	credentialplugin.Run(*p)
}
