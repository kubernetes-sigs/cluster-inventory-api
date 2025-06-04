package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/pkg/apis/clientauthentication"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"os"
	"os/exec"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/tools/plugins"
)

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

var (
	// The list of API versions we accept.
	apiVersions = map[string]schema.GroupVersion{
		clientauthenticationv1.SchemeGroupVersion.String(): clientauthenticationv1.SchemeGroupVersion,
	}
)

type Plugin struct {
	name string
}

type ExecConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Exec-based authentication provider.
	ExecProvider *clientcmdapi.ExecConfig `json:"execProvider,omitempty"`
}

func NewPlugin(name string) (plugins.PluginInterface, error) {
	return &Plugin{name: name}, nil
}

func (p *Plugin) Name() string {
	return p.name
}

func (p *Plugin) Credential(cluster *v1alpha1.ClusterProfile) (*clientauthentication.ExecCredential, error) {
	cred := &clientauthentication.ExecCredential{}

	var execConfig *ExecConfig
	for _, provider := range cluster.Status.CredentialProviders {
		if provider.Name != p.name {
			continue
		}

		execConfig := &ExecConfig{}
		err := json.Unmarshal(provider.Config.Raw, execConfig)
		if err != nil {
			return nil, err
		}
	}
	if execConfig == nil {
		return nil, fmt.Errorf("plugin is not configured")
	}

	stdout := &bytes.Buffer{}
	cmd := exec.Command(execConfig.ExecProvider.Command, execConfig.ExecProvider.Args...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	cmd.Stdout = stdout

	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	group, ok := apiVersions[execConfig.APIVersion]
	if !ok {
		return nil, fmt.Errorf("plugin does not support API version %q", execConfig.APIVersion)
	}

	_, _, err = codecs.UniversalDecoder(group).Decode(stdout.Bytes(), nil, cred)
	if err != nil {
		return nil, fmt.Errorf("decoding stdout: %v", err)
	}

	return cred, nil
}
