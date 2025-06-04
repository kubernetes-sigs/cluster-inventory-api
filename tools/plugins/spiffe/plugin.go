package spiffe

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	"github.com/spiffe/spire/pkg/common/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/clientauthentication"
	"net"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/tools/plugins"
)

// spiffe plugin requires each spoke cluster to use a single spire server as an oidc provider,
// and the hub cluster install the spire agent to register all service account to the spire
// server. Such that the controller on the hub cluster can fetch token from the spire server
// to be authenticated to the spoke cluster.
type Plugin struct{}

const pluginName = "spiffe"

type SpiffeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Config Config `json:"config"`
}

type Config struct {
	Address  string `json:"address"`
	Audience string `json:"audience"`
}

func NewPlugin() (plugins.PluginInterface, error) {
	return &Plugin{}, nil
}

func (p *Plugin) Name() string {
	return pluginName
}

func (p *Plugin) Credential(cluster *v1alpha1.ClusterProfile) (*clientauthentication.ExecCredential, error) {
	var spiffeConfig *SpiffeConfig
	for _, provider := range cluster.Status.CredentialProviders {
		if provider.Name != pluginName {
			continue
		}

		spiffeConfig := &SpiffeConfig{}
		err := json.Unmarshal(provider.Config.Raw, spiffeConfig)
		if err != nil {
			return nil, err
		}
	}
	if spiffeConfig == nil {
		return nil, fmt.Errorf("no SPIFFE configured")
	}

	addrStr := spiffeConfig.Config.Address
	addr, err := net.ResolveUnixAddr("unix", addrStr)
	target, err := util.GetTargetName(addr)
	if err != nil {
		return nil, err
	}
	conn, err := util.NewGRPCClient(target)
	if err != nil {
		return nil, err
	}
	client := workload.NewSpiffeWorkloadAPIClient(conn)

	svid, err := client.FetchJWTSVID(context.Background(), &workload.JWTSVIDRequest{
		Audience: []string{spiffeConfig.Config.Audience},
	})
	if err != nil {
		return nil, err
	}
	return &clientauthentication.ExecCredential{
		Status: &clientauthentication.ExecCredentialStatus{
			Token: svid.Svids[0].Svid,
		},
	}, nil
}
