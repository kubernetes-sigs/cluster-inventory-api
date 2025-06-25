package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"os"
	"path/filepath"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentials"
	"sigs.k8s.io/yaml"
)

var _ = ginkgo.Describe("Credentials test", func() {
	var clusterName string
	var clusterManagerName string
	var tempDir string

	ginkgo.BeforeEach(func() {
		clusterName = fmt.Sprintf("cluster-%s", rand.String(5))
		clusterManagerName = fmt.Sprintf("cluster-manager-%s", rand.String(5))

		clusterProfile := &v1alpha1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Labels:    map[string]string{v1alpha1.LabelClusterManagerKey: clusterManagerName},
				Namespace: testNamespace,
			},
			Spec: v1alpha1.ClusterProfileSpec{
				DisplayName: clusterName,
				ClusterManager: v1alpha1.ClusterManager{
					Name: clusterManagerName,
				},
			},
		}

		_, err := clusterProfileClient.ApisV1alpha1().ClusterProfiles(testNamespace).Create(
			context.TODO(),
			clusterProfile,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		tempDir, err = os.MkdirTemp("", "integration-credentials-test")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		if tempDir != "" {
			err := os.RemoveAll(tempDir)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	ginkgo.It("Should get credntial by cluster profile", func() {
		cp, err := clusterProfileClient.ApisV1alpha1().ClusterProfiles(testNamespace).Get(
			context.TODO(), clusterName, metav1.GetOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		cp.Status = v1alpha1.ClusterProfileStatus{
			CredentialProviders: []v1alpha1.CredentialProvider{
				{
					Name: "provider1",
					Cluster: clientcmdv1.Cluster{
						Server:                   cfg.Host,
						CertificateAuthorityData: cfg.CAData,
					},
				},
			},
		}

		cp, err = clusterProfileClient.ApisV1alpha1().ClusterProfiles(testNamespace).UpdateStatus(
			context.TODO(),
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
		credProviderConfig := credentials.CredentialsProvider{
			Providers: []credentials.Provider{
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

		credProviderData, err := json.Marshal(credProviderConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		configFile := filepath.Join(tempDir, "test-config.json")
		err = os.WriteFile(configFile, credProviderData, 0644)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		credentialConfig, err := credentials.NewFromFile(configFile)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		config, err := credentialConfig.BuildConfigFromCP(cp)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(config).NotTo(gomega.BeNil())

		kubeClientFromCP, err := kubernetes.NewForConfig(config)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// test if the client can connect to the cluster
		_, err = kubeClientFromCP.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
})
