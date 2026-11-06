package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
	"sigs.k8s.io/cluster-inventory-api/pkg/access"
	"sigs.k8s.io/yaml"
)

var _ = ginkgo.Describe("Access config test", func() {
	var clusterName string
	var clusterManagerName string
	var tempDir string

	ginkgo.BeforeEach(func(ctx context.Context) {
		clusterName = fmt.Sprintf("cluster-%s", rand.String(5))
		clusterManagerName = fmt.Sprintf("cluster-manager-%s", rand.String(5))

		clusterProfile := &v1alpha2.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Labels:    map[string]string{v1alpha2.LabelClusterManagerKey: clusterManagerName},
				Namespace: testNamespace,
			},
			Spec: v1alpha2.ClusterProfileSpec{
				DisplayName: clusterName,
				ClusterManager: v1alpha2.ClusterManager{
					Name: clusterManagerName,
				},
			},
		}

		_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
			ctx,
			clusterProfile,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		tempDir, err = os.MkdirTemp("", "integration-access-test")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		if tempDir != "" {
			err := os.RemoveAll(tempDir)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	ginkgo.It("preserves kubelogin arguments and extension objects through v1alpha2",
		func(ctx context.Context) {
			additionalArgs := []string{
				"--oidc-issuer-url=https://id.example.com/realms/platform",
				"--oidc-client-id=cluster-a",
				"--oidc-extra-scope=email",
				"--oidc-extra-scope=profile",
			}
			argsData, err := json.Marshal(additionalArgs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			cluster := v1alpha2.Cluster{
				Server: cfg.Host,
				Extensions: []v1alpha2.NamedExtension{
					{
						Name:      "clusterprofiles.multicluster.x-k8s.io/exec/additional-args",
						Extension: runtime.RawExtension{Raw: argsData},
					},
					{
						Name:      "clusterprofiles.multicluster.x-k8s.io/exec/additional-envs",
						Extension: runtime.RawExtension{Raw: []byte(`{"CLIENT_ID":"c"}`)},
					},
					{
						Name: "client.authentication.k8s.io/exec",
						Extension: runtime.RawExtension{
							Raw: []byte(`{"audience":"x","nested":{"scopes":["read","write"]}}`),
						},
					},
				},
			}

			client := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace)
			cp, err := client.Get(ctx, clusterName, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			cp.Status.AccessProviders = []v1alpha2.AccessProvider{{Name: "provider1", Cluster: cluster}}
			_, err = client.UpdateStatus(ctx, cp, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gotV1Alpha1, err := clusterProfileClient.ApisV1alpha1().ClusterProfiles(testNamespace).Get(
				ctx, clusterName, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(gotV1Alpha1.Status.AccessProviders).To(gomega.HaveLen(1))
			gotV1Alpha2, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Get(
				ctx, clusterName, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(gotV1Alpha2.Status.AccessProviders).To(gomega.HaveLen(1))
			gotExtensions := gotV1Alpha2.Status.AccessProviders[0].Cluster.Extensions
			gomega.Expect(gotExtensions).To(gomega.HaveLen(len(cluster.Extensions)))
			for i, extension := range gotExtensions {
				gomega.Expect(extension.Name).To(gomega.Equal(cluster.Extensions[i].Name))
				gomega.Expect(extension.Extension.Raw).To(gomega.MatchJSON(cluster.Extensions[i].Extension.Raw))
			}
			gomega.Expect(gotV1Alpha1.Status.AccessProviders[0].Cluster.Extensions).To(gomega.Equal(
				access.ToClientcmdCluster(gotV1Alpha2.Status.AccessProviders[0].Cluster).Extensions))

			accessCfg := access.New([]access.Provider{{
				Name: "provider1",
				ExecConfig: &clientcmdapi.ExecConfig{
					APIVersion: "client.authentication.k8s.io/v1",
					Command:    "kubectl",
					Args:       []string{"oidc-login", "get-token"},
				},
				ProfileSourcedCLIArgsPolicy: access.ProfileSourcedCLIArgsPolicyAppend,
				ProfileSourcedEnvVarsPolicy: access.ProfileSourcedEnvVarsPolicyReplace,
			}})
			restConfig, err := accessCfg.BuildConfigFromCP(gotV1Alpha2)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(restConfig.ExecProvider.Command).To(gomega.Equal("kubectl"))
			gomega.Expect(restConfig.ExecProvider.Args).To(gomega.Equal(
				append([]string{"oidc-login", "get-token"}, additionalArgs...)))
			gomega.Expect(restConfig.ExecProvider.Env).To(gomega.Equal([]clientcmdapi.ExecEnvVar{
				{Name: "CLIENT_ID", Value: "c"},
			}))
			pluginConfig, ok := restConfig.ExecProvider.Config.(*runtime.Unknown)
			gomega.Expect(ok).To(gomega.BeTrue())
			gomega.Expect(pluginConfig.Raw).To(gomega.MatchJSON(cluster.Extensions[2].Extension.Raw))
		},
	)

	ginkgo.It("Should get access config by cluster profile", func(ctx context.Context) {
		cp, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Get(
			ctx, clusterName, metav1.GetOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		cp.Status = v1alpha2.ClusterProfileStatus{
			AccessProviders: []v1alpha2.AccessProvider{
				{
					Name: "provider1",
					Cluster: v1alpha2.Cluster{
						Server:                   cfg.Host,
						CertificateAuthorityData: cfg.CAData,
					},
				},
			},
		}

		cp, err = clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
			ctx,
			cp,
			metav1.UpdateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// read creds in existing rest.Config and put into execCredential.
		credConfig := clientauthenticationv1.ExecCredential{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "client.authentication.k8s.io/v1",
				Kind:       "ExecCredential",
			},
			Status: &clientauthenticationv1.ExecCredentialStatus{
				ClientCertificateData: string(cfg.CertData),
				ClientKeyData:         string(cfg.KeyData),
			},
		}

		credData, err := yaml.Marshal(credConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		credFile := filepath.Join(tempDir, "test-cred.yaml")
		err = os.WriteFile(credFile, credData, 0644)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Create provider configuration file
		providerConfig := access.Config{
			Providers: []access.Provider{
				{
					Name: "provider1",
					ExecConfig: &clientcmdapi.ExecConfig{
						APIVersion:         "client.authentication.k8s.io/v1",
						Command:            "cat",
						Args:               []string{credFile},
						ProvideClusterInfo: true,
					},
				},
			},
		}

		providerData, err := json.Marshal(providerConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		configFile := filepath.Join(tempDir, "test-config.json")
		err = os.WriteFile(configFile, providerData, 0644)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		accessCfg, err := access.NewFromFile(configFile)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		config, err := accessCfg.BuildConfigFromCP(cp)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(config).NotTo(gomega.BeNil())

		kubeClientFromCP, err := kubernetes.NewForConfig(config)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// test if the client can connect to the cluster
		_, err = kubeClientFromCP.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
})
