package spiffe

import (
	"context"
	"github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	"github.com/spiffe/spire/pkg/common/util"
	"k8s.io/client-go/pkg/apis/clientauthentication"
	"net"
	"os"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/tools/plugins"
)

// spiffe plugin requires each spoke cluster to use a single spire server as an oidc provider,
// and the hub cluster install the spire agent to register all service account to the spire
// server. Such that the controller on the hub cluster can fetch token from the spire server
// to be authenticated to the spoke cluster.
type Plugin struct {
	client workload.SpiffeWorkloadAPIClient
}

func NewPlugin() (plugins.PluginInterface, error) {
	addrStr := os.Getenv("SPIFFE_ADDRESS")
	addr, err := net.ResolveUnixAddr("unix", addrStr)
	target, err := util.GetTargetName(addr)
	if err != nil {
		return nil, err
	}
	conn, err := util.NewGRPCClient(target)
	if err != nil {
		return nil, err
	}

	return &Plugin{client: workload.NewSpiffeWorkloadAPIClient(conn)}, nil
}

func (p *Plugin) Name() string {
	return "spiffe"
}

func (p *Plugin) Credential(_ *v1alpha1.ClusterProfile) (*clientauthentication.ExecCredential, error) {
	svid, err := p.client.FetchJWTSVID(context.Background(), &workload.JWTSVIDRequest{
		Audience: []string{"kube"},
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
