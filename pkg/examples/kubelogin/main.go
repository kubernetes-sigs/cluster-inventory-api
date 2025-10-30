package main

import (
	"log"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"

	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentials"
)

func main() {
	providers := []credentials.Provider{
		{
			Name: "aks-workload-identity",
			ExecConfig: &clientcmdapi.ExecConfig{
				Command: "kubelogin",
				Args: []string{
					"get-token",
					"--login",
					"workloadidentity",
					"--federated-token-file",
					// The well-known path where AKS mounts the service account token as a projected volume.
					//
					// This is an application-specific information and it can be configured to
					// a different path if needed.
					"/var/run/secrets/tokens/azure-identity-token",
				},
				Env:                []clientcmdapi.ExecEnvVar{},
				APIVersion:         "client.authentication.k8s.io/v1beta1",
				ProvideClusterInfo: false,
				InteractiveMode:    clientcmdapi.NeverExecInteractiveMode,
			},
			AdditionalCLIArgEnvVarExtensionFlag: credentials.AdditionalCLIArgEnvVarExtensionFlagAllow,
		},
	}
	cps := credentials.New(providers)

	// The additional arguments are cluster-specific information.
	additionalArgs := []string{
		"--tenant-id", "TENANT_ID",
		"--authority-host", "https://login.microsoftonline.com/",
		// The kubelogin plugin already knows the scopes for AKS; no need to specify it explicitly.
	}
	additionalArgsYAML, err := yaml.Marshal(additionalArgs)
	if err != nil {
		log.Fatalf("failed to marshal additional args")
	}

	additionalEnvVars := map[string]string{
		"AZURE_CLIENT_ID": "CLIENT_ID",
	}
	additionalEnvVarsYAML, err := yaml.Marshal(additionalEnvVars)
	if err != nil {
		log.Fatalf("failed to marshal additional env vars")
	}

	profile := &v1alpha1.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bravelion",
			Namespace: "fleet-system",
		},
		Spec: v1alpha1.ClusterProfileSpec{
			DisplayName: "bravelion",
			ClusterManager: v1alpha1.ClusterManager{
				Name: "kubefleet",
			},
		},
		Status: v1alpha1.ClusterProfileStatus{
			CredentialProviders: []v1alpha1.CredentialProvider{
				{
					Name: "aks-workload-identity",
					Cluster: clientcmdapiv1.Cluster{
						Server:                   "https://bravelion.hcp.eastus.azmk8s.io:443",
						CertificateAuthorityData: []byte(""),
						Extensions: []clientcmdapiv1.NamedExtension{
							{
								Name: "multicluster.x-k8s.io/clusterprofiles/auth/exec/additional-args",
								Extension: runtime.RawExtension{
									Raw: additionalArgsYAML,
								},
							},
							{
								Name: "multicluster.x-k8s.io/clusterprofiles/auth/exec/additional-envs",
								Extension: runtime.RawExtension{
									Raw: additionalEnvVarsYAML,
								},
							},
						},
					},
				},
			},
		},
	}

	restConfig, err := cps.BuildConfigFromCP(profile)
	if err != nil {
		log.Fatalf("Failed to prepare REST config: %v", err)
	}

	// The generated REST config can be used to build a Kubernetes client.
	//
	// It will invoke the kubelogin plugin as follows:
	//
	// kubelogin get-token \
	//     --login workloadidentity \
	//     --federated-token-file /var/run/secrets/tokens/azure-identity-token \
	//     --tenant-id TENANT_ID \
	//     --client-id CLIENT_ID \
	//     --authority-host https://login.microsoftonline.com/
	log.Printf("Prepared REST config:\n%+v", restConfig)
	log.Printf("CLI Args: %s", restConfig.ExecProvider.Args)
	log.Printf("Env Vars: %+v", restConfig.ExecProvider.Env)
}
