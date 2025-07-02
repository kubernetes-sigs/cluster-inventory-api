/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package credentials

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

func TestCredentials(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Credentials Package Suite")
}

var _ = ginkgo.Describe("CredentialsProvider", func() {
	var (
		tempDir             string
		testProviders       []Provider
		credentialsProvider *CredentialsProvider
	)

	ginkgo.BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "credentials-test")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		testProviders = []Provider{
			{
				Name: "test-provider-1",
				ExecConfig: &clientcmdapi.ExecConfig{
					Command:    "test-command-1",
					Args:       []string{"arg1", "arg2"},
					APIVersion: "client.authentication.k8s.io/v1beta1",
				},
			},
			{
				Name: "test-provider-2",
				ExecConfig: &clientcmdapi.ExecConfig{
					Command:    "test-command-2",
					Args:       []string{"arg3"},
					APIVersion: "client.authentication.k8s.io/v1beta1",
				},
			},
		}
		credentialsProvider = New(testProviders)
	})

	ginkgo.AfterEach(func() {
		if tempDir != "" {
			err := os.RemoveAll(tempDir)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	ginkgo.Describe("New", func() {
		ginkgo.It("should create a new CredentialsProvider with provided providers", func() {
			providers := []Provider{
				{Name: "provider1", ExecConfig: &clientcmdapi.ExecConfig{Command: "cmd1"}},
				{Name: "provider2", ExecConfig: &clientcmdapi.ExecConfig{Command: "cmd2"}},
			}
			cp := New(providers)
			gomega.Expect(cp).NotTo(gomega.BeNil())
			gomega.Expect(cp.Providers).To(gomega.HaveLen(2))
			gomega.Expect(cp.Providers[0].Name).To(gomega.Equal("provider1"))
			gomega.Expect(cp.Providers[1].Name).To(gomega.Equal("provider2"))
		})

		ginkgo.It("should create a CredentialsProvider with empty providers", func() {
			cp := New([]Provider{})
			gomega.Expect(cp).NotTo(gomega.BeNil())
			gomega.Expect(cp.Providers).To(gomega.HaveLen(0))
		})

		ginkgo.It("should create a CredentialsProvider with nil providers", func() {
			cp := New(nil)
			gomega.Expect(cp).NotTo(gomega.BeNil())
			gomega.Expect(cp.Providers).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("SetupProviderFileFlag", func() {
		ginkgo.It("should define a command-line flag and return a pointer to the string", func() {
			// Reset flag package for clean test
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			filePath := SetupProviderFileFlag()
			gomega.Expect(filePath).NotTo(gomega.BeNil())
			gomega.Expect(*filePath).To(gomega.Equal("clusterprofile-provider-file.json"))

			// Verify the flag was registered
			flagSet := flag.CommandLine
			flagValue := flagSet.Lookup("clusterprofile-provider-file")
			gomega.Expect(flagValue).NotTo(gomega.BeNil())
			gomega.Expect(flagValue.DefValue).To(gomega.Equal("clusterprofile-provider-file.json"))
		})
	})

	ginkgo.Describe("NewFromFile", func() {
		ginkgo.It("should successfully read and unmarshal a valid JSON file", func() {
			// Create a test JSON file
			testData := CredentialsProvider{
				Providers: []Provider{
					{
						Name: "gkeFleet",
						ExecConfig: &clientcmdapi.ExecConfig{
							APIVersion:         "client.authentication.k8s.io/v1beta1",
							Command:            "gke-gcloud-auth-plugin",
							ProvideClusterInfo: true,
						},
					},
				},
			}

			jsonData, err := json.Marshal(testData)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			testFile := filepath.Join(tempDir, "test-config.json")
			err = os.WriteFile(testFile, jsonData, 0644)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Test reading the file
			cp, err := NewFromFile(testFile)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cp).NotTo(gomega.BeNil())
			gomega.Expect(cp.Providers).To(gomega.HaveLen(1))
			gomega.Expect(cp.Providers[0].Name).To(gomega.Equal("gkeFleet"))
			gomega.Expect(cp.Providers[0].ExecConfig.Command).To(gomega.Equal("gke-gcloud-auth-plugin"))
		})

		ginkgo.It("should return an error when file does not exist", func() {
			nonExistentFile := filepath.Join(tempDir, "non-existent.json")
			cp, err := NewFromFile(nonExistentFile)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(cp).To(gomega.BeNil())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("failed to read credentials file"))
		})

		ginkgo.It("should return an error when file contains invalid JSON", func() {
			invalidJSONFile := filepath.Join(tempDir, "invalid.json")
			err := os.WriteFile(invalidJSONFile, []byte("invalid json content"), 0644)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			cp, err := NewFromFile(invalidJSONFile)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(cp).To(gomega.BeNil())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("failed to unmarshal credential proviers"))
		})

		ginkgo.It("should handle empty JSON file", func() {
			emptyJSONFile := filepath.Join(tempDir, "empty.json")
			err := os.WriteFile(emptyJSONFile, []byte("{}"), 0644)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			cp, err := NewFromFile(emptyJSONFile)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cp).NotTo(gomega.BeNil())
			gomega.Expect(cp.Providers).To(gomega.BeEmpty())
		})
	})

	ginkgo.Describe("getExecConfigFromConfig", func() {
		ginkgo.It("should return the correct ExecConfig for existing provider", func() {
			execConfig := credentialsProvider.getExecConfigFromConfig("test-provider-1")
			gomega.Expect(execConfig).NotTo(gomega.BeNil())
			gomega.Expect(execConfig.Command).To(gomega.Equal("test-command-1"))
			gomega.Expect(execConfig.Args).To(gomega.Equal([]string{"arg1", "arg2"}))
		})

		ginkgo.It("should return nil for non-existing provider", func() {
			execConfig := credentialsProvider.getExecConfigFromConfig("non-existent-provider")
			gomega.Expect(execConfig).To(gomega.BeNil())
		})

		ginkgo.It("should return nil for empty provider name", func() {
			execConfig := credentialsProvider.getExecConfigFromConfig("")
			gomega.Expect(execConfig).To(gomega.BeNil())
		})

		ginkgo.It("should handle CredentialsProvider with no providers", func() {
			emptyCP := New([]Provider{})
			execConfig := emptyCP.getExecConfigFromConfig("any-provider")
			gomega.Expect(execConfig).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("getProviderFromClusterProfile", func() {
		var clusterProfile *v1alpha1.ClusterProfile

		ginkgo.BeforeEach(func() {
			clusterProfile = &v1alpha1.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1alpha1.ClusterProfileStatus{
					CredentialProviders: []v1alpha1.CredentialProvider{
						{
							Name: "test-provider-1",
							Cluster: clientcmdv1.Cluster{
								Server:                   "https://test-server-1.com",
								CertificateAuthorityData: []byte("test-ca-data-1"),
							},
						},
						{
							Name: "test-provider-2",
							Cluster: clientcmdv1.Cluster{
								Server:                   "https://test-server-2.com",
								CertificateAuthorityData: []byte("test-ca-data-2"),
							},
						},
						{
							Name: "unsupported-provider",
							Cluster: clientcmdv1.Cluster{
								Server: "https://unsupported-server.com",
							},
						},
					},
				},
			}
		})

		ginkgo.It("should return the first matching provider", func() {
			provider := credentialsProvider.getProviderFromClusterProfile(clusterProfile)
			gomega.Expect(provider).NotTo(gomega.BeNil())
			gomega.Expect(provider.Name).To(gomega.Equal("test-provider-1"))
			gomega.Expect(provider.Cluster.Server).To(gomega.Equal("https://test-server-1.com"))
		})

		ginkgo.It("should return nil when no matching provider is found", func() {
			// Create a CredentialsProvider with providers that don't match the ClusterProfile
			mismatchedCP := New([]Provider{
				{Name: "different-provider", ExecConfig: &clientcmdapi.ExecConfig{Command: "cmd"}},
			})
			provider := mismatchedCP.getProviderFromClusterProfile(clusterProfile)
			gomega.Expect(provider).To(gomega.BeNil())
		})

		ginkgo.It("should handle ClusterProfile with no credential providers", func() {
			emptyClusterProfile := &v1alpha1.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-cluster"},
				Status:     v1alpha1.ClusterProfileStatus{},
			}
			provider := credentialsProvider.getProviderFromClusterProfile(emptyClusterProfile)
			gomega.Expect(provider).To(gomega.BeNil())
		})

		ginkgo.It("should return a deep copy of the provider", func() {
			provider := credentialsProvider.getProviderFromClusterProfile(clusterProfile)
			gomega.Expect(provider).NotTo(gomega.BeNil())

			// Modify the original cluster profile
			originalServer := clusterProfile.Status.CredentialProviders[0].Cluster.Server
			clusterProfile.Status.CredentialProviders[0].Cluster.Server = "modified-server"

			// The returned provider should not be affected
			gomega.Expect(provider.Cluster.Server).To(gomega.Equal(originalServer))
			gomega.Expect(provider.Cluster.Server).NotTo(gomega.Equal("modified-server"))
		})
	})

	ginkgo.Describe("convertCluster", func() {
		ginkgo.It("should convert clientcmdv1.Cluster to clientauthentication.Cluster", func() {
			inputCluster := clientcmdv1.Cluster{
				Server:                   "https://test-server.com",
				TLSServerName:            "test-tls-server",
				InsecureSkipTLSVerify:    true,
				CertificateAuthorityData: []byte("test-ca-data"),
				ProxyURL:                 "http://proxy.example.com",
				DisableCompression:       true,
			}

			result := convertCluster(inputCluster)
			gomega.Expect(result).NotTo(gomega.BeNil())
			gomega.Expect(result.Server).To(gomega.Equal("https://test-server.com"))
			gomega.Expect(result.TLSServerName).To(gomega.Equal("test-tls-server"))
			gomega.Expect(result.InsecureSkipTLSVerify).To(gomega.BeTrue())
			gomega.Expect(result.CertificateAuthorityData).To(gomega.Equal([]byte("test-ca-data")))
			gomega.Expect(result.ProxyURL).To(gomega.Equal("http://proxy.example.com"))
			gomega.Expect(result.DisableCompression).To(gomega.BeTrue())
		})

		ginkgo.It("should handle empty cluster", func() {
			inputCluster := clientcmdv1.Cluster{}
			result := convertCluster(inputCluster)
			gomega.Expect(result).NotTo(gomega.BeNil())
			gomega.Expect(result.Server).To(gomega.BeEmpty())
			gomega.Expect(result.TLSServerName).To(gomega.BeEmpty())
			gomega.Expect(result.InsecureSkipTLSVerify).To(gomega.BeFalse())
			gomega.Expect(result.CertificateAuthorityData).To(gomega.BeNil())
			gomega.Expect(result.ProxyURL).To(gomega.BeEmpty())
			gomega.Expect(result.DisableCompression).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("BuildConfigFromCP", func() {
		var clusterProfile *v1alpha1.ClusterProfile

		ginkgo.BeforeEach(func() {
			clusterProfile = &v1alpha1.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1alpha1.ClusterProfileStatus{
					CredentialProviders: []v1alpha1.CredentialProvider{
						{
							Name: "test-provider-1",
							Cluster: clientcmdv1.Cluster{
								Server:                   "https://test-server.com",
								CertificateAuthorityData: []byte("test-ca-data"),
								ProxyURL:                 "http://proxy.example.com",
							},
						},
					},
				},
			}
		})

		ginkgo.It("should return an error when no matching provider is found", func() {
			// Create a CredentialsProvider with providers that don't match the ClusterProfile
			mismatchedCP := New([]Provider{
				{Name: "different-provider", ExecConfig: &clientcmdapi.ExecConfig{Command: "cmd"}},
			})
			config, err := mismatchedCP.BuildConfigFromCP(clusterProfile)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(config).To(gomega.BeNil())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("no matching provider found for cluster profile"))
		})

		ginkgo.It("should return an error when no exec config is found", func() {
			// Create a CredentialsProvider with matching provider name but no ExecConfig
			noExecCP := New([]Provider{
				{Name: "test-provider-1", ExecConfig: nil},
			})
			config, err := noExecCP.BuildConfigFromCP(clusterProfile)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(config).To(gomega.BeNil())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("no exec credentials found for provider"))
		})

		ginkgo.It("should build config successfully", func() {
			cred := clientauthenticationv1.ExecCredential{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "client.authentication.k8s.io/v1",
					Kind:       "ExecCredential",
				},
				Status: &clientauthenticationv1.ExecCredentialStatus{
					Token: "test-token",
				},
			}
			jsonData, err := json.Marshal(cred)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testFile := filepath.Join(tempDir, "test-config.json")
			err = os.WriteFile(testFile, jsonData, 0644)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			execCP := New([]Provider{
				{Name: "test-provider-1", ExecConfig: &clientcmdapi.ExecConfig{
					APIVersion: "client.authentication.k8s.io/v1",
					Command:    "cat",
					Args:       []string{testFile},
				}},
			})

			config, err := execCP.BuildConfigFromCP(clusterProfile)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(config).NotTo(gomega.BeNil())
			gomega.Expect(config.Host).To(gomega.Equal("https://test-server.com"))
			gomega.Expect(config.TLSClientConfig.CAData).To(gomega.Equal([]byte("test-ca-data")))
		})
	})
})
