package ocm

import (
	"context"
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/clientauthentication"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/tools/plugins"
)

// Plugin of OCM stores token from the managedcluster in the namespace
// which has the same name of the cluster. It needs to connect
// to the hub cluster to get the secret.
type Plugin struct {
	client kubernetes.Interface
}

func NewPlugin() (plugins.PluginInterface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Plugin{client: client}, nil
}

func (p *Plugin) Name() string {
	return "ocm"
}

func (p *Plugin) Credential(cluster *v1alpha1.ClusterProfile) (*clientauthentication.ExecCredential, error) {
	tokenSecret, err := p.client.CoreV1().Secrets(cluster.Name).Get(context.Background(), "token", v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("faield to get token secret for cluster %s. %v", cluster.Name, err)
	}

	if len(tokenSecret.Data["token"]) == 0 {
		return nil, fmt.Errorf("token secret for cluster %s does not have a token", cluster.Name)
	}

	return &clientauthentication.ExecCredential{
		Status: &clientauthentication.ExecCredentialStatus{
			Token: string(tokenSecret.Data["token"]),
		},
	}, nil
}
