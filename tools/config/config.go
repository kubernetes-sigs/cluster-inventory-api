package config

import (
	"encoding/json"
	"fmt"
	clientauthenticationapi "k8s.io/client-go/pkg/apis/clientauthentication"
	"k8s.io/client-go/plugin/pkg/client/auth/exec"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"net/http"
	"net/url"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

func getProviderFromClusterProfile(cluster *v1alpha1.ClusterProfile, providerName string) (v1alpha1.ProviderConfig, bool) {
	for _, provider := range cluster.Status.CredentialProviders {
		if provider.Name == providerName {
			return provider.Config, true
		}
	}
	return v1alpha1.ProviderConfig{}, false
}

func configToExecCluster(config v1alpha1.ProviderConfig) (*clientauthenticationapi.Cluster, error) {
	outConfig := &clientauthenticationapi.Cluster{
		Server:                   config.Server,
		TLSServerName:            config.TLSServerName,
		InsecureSkipTLSVerify:    config.InsecureSkipTLSVerify,
		CertificateAuthorityData: config.CertificateAuthorityData,
		ProxyURL:                 config.ProxyURL,
		DisableCompression:       config.DisableCompression,
	}

	return outConfig, nil
}

// BuildConfigFromClusterProfile is to build the rest.Config to init the client.
func BuildConfigFromClusterProfile(cluster *v1alpha1.ClusterProfile, providerName string) (*rest.Config, error) {
	provider, found := getProviderFromClusterProfile(cluster, providerName)
	if !found {
		return nil, fmt.Errorf("no provider found for cluster profile %q", cluster.Name)
	}

	execCluster, err := configToExecCluster(provider)
	if err != nil {
		return nil, err
	}

	// If it is not exec Config, we have to find a way to fallback to a different path.
	execConfig := &clientcmdapi.ExecConfig{}
	err = json.Unmarshal(provider.Config.Raw, execConfig)
	if err != nil {
		return nil, err
	}

	a, err := exec.GetAuthenticator(execConfig, execCluster)
	if err != nil {
		return nil, err
	}

	config := &rest.Config{
		Host: provider.Server,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: provider.CertificateAuthorityData,
		},
		Proxy: func(request *http.Request) (*url.URL, error) {
			return url.Parse(provider.ProxyURL)
		},
	}

	transportConfig, err := config.TransportConfig()
	if err := a.UpdateTransportConfig(transportConfig); err != nil {
		return nil, err
	}

	return config, nil
}
