package config

import (
	"fmt"
	"net/http"
	"net/url"

	"k8s.io/client-go/pkg/apis/clientauthentication"
	"k8s.io/client-go/plugin/pkg/client/auth/exec"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

type Provider struct {
	Name string
	ExecConfig *clientcmdapi.ExecConfig
}

type CredentialsProvider struct {
	providers []Provider
}

func New(providers []Provider) *CredentialsProvider {
	return &CredentialsProvider{
		providers: providers,
	}
}

func (cp *CredentialsProvider) BuildConfigFromCP(clusterprofile *v1alpha1.ClusterProfile)(*rest.Config, error) {
	// 1. obtain the correct provider from the CP
	provider := cp.getProviderFromClusterProfile(clusterprofile)
	if provider == nil {
		return nil, fmt.Errorf("no matching provider found for cluster profile %q", clusterprofile.Name)
	}
	cluster := convertCluster(provider.Cluster)

	// 2. Get Exec Config
	execConfig := cp.getExecConfigFromConfig(provider.Name)
	if execConfig == nil {
		return nil, fmt.Errorf("no exec config found for provider %q", provider.Name)
	}

	// 2. call exec
	a, err := exec.GetAuthenticator(execConfig, cluster)
	if err != nil {
		return nil, err
	}

	// 3. build resulting rest.Config
	config := &rest.Config{
		Host: cluster.Server,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: cluster.CertificateAuthorityData,
		},
		Proxy: func(request *http.Request) (*url.URL, error) {
			return url.Parse(cluster.ProxyURL)
		},
	}

	transportConfig, err := config.TransportConfig()
	if err := a.UpdateTransportConfig(transportConfig); err != nil {
		return nil, err
	}

	return config, nil
}

func (cp *CredentialsProvider) getExecConfigFromConfig(providerName string) (*clientcmdapi.ExecConfig) {
	for _, provider := range cp.providers {
		if provider.Name == providerName {
			return provider.ExecConfig
		}
	}
	return nil
}

func (cp *CredentialsProvider) getProviderFromClusterProfile(cluster *v1alpha1.ClusterProfile) *v1alpha1.CredentialProvider {
	cpProviderTypes := map[string]*v1alpha1.CredentialProvider{}

	for _, provider := range cluster.Status.CredentialProviders {
		newProvider := provider.DeepCopy()
		cpProviderTypes[provider.Name] = newProvider
	}

	// we return the first provider that the CP supports.
	for _, providerType := range(cp.providers) {
		if provider, found := cpProviderTypes[providerType.Name]; found {
			return provider
		}
	}
	return nil
}

func convertCluster(cluster clientcmdv1.Cluster) *clientauthentication.Cluster {
	return &clientauthentication.Cluster{
		Server:                   cluster.Server,
		TLSServerName:            cluster.TLSServerName,
		InsecureSkipTLSVerify:    cluster.InsecureSkipTLSVerify,
		//CertificateAuthority:     cluster.CertificateAuthority,
		CertificateAuthorityData: cluster.CertificateAuthorityData,
		ProxyURL:                 cluster.ProxyURL,
		DisableCompression:       cluster.DisableCompression,
	}
	// missing extensions and config? Also CertificateAuthority?
}
