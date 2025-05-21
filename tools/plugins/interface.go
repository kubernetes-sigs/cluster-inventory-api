package plugins

import (
	"k8s.io/client-go/pkg/apis/clientauthentication"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

type PluginInterface interface {
	// returns the name of the plugins
	Name() string

	// Credential returns the authentication information to connect to the cluster.
	Credential(cluster *v1alpha1.ClusterProfile) (*clientauthentication.ExecCredential, error)
}
