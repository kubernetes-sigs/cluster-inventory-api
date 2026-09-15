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

package integration

import (
	"context"
	"fmt"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/dynamic"

	cpv1alpha2 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
)

var _ = ginkgo.Describe("ClusterProfileAPI test", func() {
	var clusterName string
	var clusterManagerName string

	ginkgo.BeforeEach(func() {
		clusterName = fmt.Sprintf("cluster-%s", rand.String(5))
		clusterManagerName = fmt.Sprintf("cluster-manager-%s", rand.String(5))
	})

	ginkgo.It("Should create a ClusterProfile", func(ctx context.Context) {
		clusterProfile := &cpv1alpha2.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:   clusterName,
				Labels: map[string]string{cpv1alpha2.LabelClusterManagerKey: clusterManagerName},
			},
			Spec: cpv1alpha2.ClusterProfileSpec{
				DisplayName: clusterName,
				ClusterManager: cpv1alpha2.ClusterManager{
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
	})

	ginkgo.DescribeTable("Should reject a ClusterProfile with a cluster manager name that is not a valid label value",
		func(ctx context.Context, invalidName string) {
			clusterProfile := &cpv1alpha2.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{Name: clusterName},
				Spec: cpv1alpha2.ClusterProfileSpec{
					ClusterManager: cpv1alpha2.ClusterManager{
						Name: invalidName,
					},
				},
			}

			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
				ctx,
				clusterProfile,
				metav1.CreateOptions{},
			)
			gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
		},
		ginkgo.Entry("empty name", ""),
		ginkgo.Entry("leading dash", "-leading-dash"),
		ginkgo.Entry("trailing dash", "trailing-dash-"),
		ginkgo.Entry("slash", "slash/name"),
		ginkgo.Entry("64 characters", strings.Repeat("a", 64)),
	)

	ginkgo.It(
		"Should accept a ClusterProfile with a cluster manager name that is a valid label value",
		func(ctx context.Context) {
			clusterProfile := &cpv1alpha2.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{Name: clusterName},
				Spec: cpv1alpha2.ClusterProfileSpec{
					ClusterManager: cpv1alpha2.ClusterManager{
						Name: "Cluster_Manager-1.x",
					},
				},
			}

			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
				ctx,
				clusterProfile,
				metav1.CreateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		},
	)

	ginkgo.It("Should update the ClusterProfile status", func(ctx context.Context) {
		clusterProfile := &cpv1alpha2.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:   clusterName,
				Labels: map[string]string{cpv1alpha2.LabelClusterManagerKey: clusterManagerName},
			},
			Spec: cpv1alpha2.ClusterProfileSpec{
				DisplayName: clusterName,
				ClusterManager: cpv1alpha2.ClusterManager{
					Name: clusterManagerName,
				},
			},
		}

		clusterProfile, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
			ctx,
			clusterProfile,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		newClusterProfile := clusterProfile.DeepCopy()
		newClusterProfile.Status.Version.Kubernetes = "1.29.0"
		newClusterProfile.Status.Properties = []cpv1alpha2.Property{{Name: "n1", Value: "v1"}}
		meta.SetStatusCondition(&newClusterProfile.Status.Conditions, metav1.Condition{
			Type:    cpv1alpha2.ClusterConditionControlPlaneHealthy,
			Status:  metav1.ConditionTrue,
			Reason:  "Reason",
			Message: "Message",
		})

		_, err = clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
			ctx,
			newClusterProfile,
			metav1.UpdateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.It("Should reject a ClusterProfile without a cluster manager", func(ctx context.Context) {
		dynamicClient, err := dynamic.NewForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		clusterProfile := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": cpv1alpha2.GroupVersion.String(),
			"kind":       cpv1alpha2.ClusterProfileKind,
			"metadata":   map[string]interface{}{"name": clusterName},
			"spec":       map[string]interface{}{"displayName": clusterName},
		}}

		_, err = dynamicClient.Resource(cpv1alpha2.ClusterProfileSchemeGroupVersionResource).
			Namespace(testNamespace).Create(ctx, clusterProfile, metav1.CreateOptions{})
		gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
	})

	ginkgo.It("Should reject an update changing the cluster manager name", func(ctx context.Context) {
		clusterProfile, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
			ctx,
			&cpv1alpha2.ClusterProfile{
				ObjectMeta: metav1.ObjectMeta{Name: clusterName},
				Spec: cpv1alpha2.ClusterProfileSpec{
					ClusterManager: cpv1alpha2.ClusterManager{
						Name: clusterManagerName,
					},
				},
			},
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		clusterProfile.Spec.ClusterManager.Name = clusterManagerName + "-changed"
		_, err = clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Update(
			ctx,
			clusterProfile,
			metav1.UpdateOptions{},
		)
		gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())

		unchanged, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Get(
			ctx, clusterProfile.Name, metav1.GetOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(unchanged.Spec.ClusterManager.Name).To(gomega.Equal(clusterManagerName))
	})

	ginkgo.It("Should isolate the spec from status updates and the status from spec updates",
		func(ctx context.Context) {
			clusterProfile, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
				ctx,
				&cpv1alpha2.ClusterProfile{
					ObjectMeta: metav1.ObjectMeta{Name: clusterName},
					Spec: cpv1alpha2.ClusterProfileSpec{
						DisplayName: clusterName,
						ClusterManager: cpv1alpha2.ClusterManager{
							Name: clusterManagerName,
						},
					},
				},
				metav1.CreateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			newClusterProfile := clusterProfile.DeepCopy()
			newClusterProfile.Spec.DisplayName = clusterName + "-via-status"
			newClusterProfile.Status.Version.Kubernetes = "1.35.0"

			afterStatus, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				newClusterProfile,
				metav1.UpdateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(afterStatus.Spec.DisplayName).To(gomega.Equal(clusterName))
			gomega.Expect(afterStatus.Status.Version.Kubernetes).To(gomega.Equal("1.35.0"))

			afterStatus.Spec.DisplayName = clusterName + "-renamed"
			afterStatus.Status.Version.Kubernetes = "0.0.0-via-spec"

			afterUpdate, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Update(
				ctx,
				afterStatus,
				metav1.UpdateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(afterUpdate.Spec.DisplayName).To(gomega.Equal(clusterName + "-renamed"))
			gomega.Expect(afterUpdate.Status.Version.Kubernetes).To(gomega.Equal("1.35.0"))
		},
	)

	ginkgo.DescribeTable("Should reject a status condition violating metav1.Condition validation",
		func(ctx context.Context, condition metav1.Condition) {
			clusterProfile, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
				ctx,
				&cpv1alpha2.ClusterProfile{
					ObjectMeta: metav1.ObjectMeta{Name: clusterName},
					Spec: cpv1alpha2.ClusterProfileSpec{
						ClusterManager: cpv1alpha2.ClusterManager{
							Name: clusterManagerName,
						},
					},
				},
				metav1.CreateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			clusterProfile.Status.Conditions = []metav1.Condition{condition}
			_, err = clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				clusterProfile,
				metav1.UpdateOptions{},
			)
			gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
		},
		ginkgo.Entry("a status other than True, False or Unknown", metav1.Condition{
			Type:               cpv1alpha2.ClusterConditionControlPlaneHealthy,
			Status:             metav1.ConditionStatus("Degraded"),
			Reason:             "Reason",
			LastTransitionTime: metav1.Now(),
		}),
		ginkgo.Entry("an empty reason", metav1.Condition{
			Type:               cpv1alpha2.ClusterConditionControlPlaneHealthy,
			Status:             metav1.ConditionTrue,
			Reason:             "",
			LastTransitionTime: metav1.Now(),
		}),
	)

	ginkgo.Context("access provider validation", func() {
		var clusterProfile *cpv1alpha2.ClusterProfile

		ginkgo.BeforeEach(func(ctx context.Context) {
			var err error
			clusterProfile, err = clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).Create(
				ctx,
				&cpv1alpha2.ClusterProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:   clusterName,
						Labels: map[string]string{cpv1alpha2.LabelClusterManagerKey: clusterManagerName},
					},
					Spec: cpv1alpha2.ClusterProfileSpec{
						DisplayName: clusterName,
						ClusterManager: cpv1alpha2.ClusterManager{
							Name: clusterManagerName,
						},
					},
				},
				metav1.CreateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should accept a valid access provider", func(ctx context.Context) {
			newClusterProfile := clusterProfile.DeepCopy()
			newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{
				AccessProviders: []cpv1alpha2.AccessProvider{{
					Name: "kubeconfig",
					Cluster: cpv1alpha2.Cluster{
						Server:                   "https://cluster.example.com:6443",
						CertificateAuthorityData: []byte("ca-data"),
						ProxyURL:                 "socks5://proxy.example.com:1080",
						Extensions: []cpv1alpha2.NamedExtension{
							{
								Name: "client.authentication.k8s.io/exec",
								Extension: runtime.RawExtension{
									Raw: []byte(`{"audience":"cluster.example.com"}`),
								},
							},
							{
								Name: "example.com/metadata",
								Extension: runtime.RawExtension{
									Raw: []byte(`{"region":"us-east-1"}`),
								},
							},
						},
					},
				}},
			}
			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				newClusterProfile,
				metav1.UpdateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should reject duplicate cluster extension names", func(ctx context.Context) {
			newClusterProfile := clusterProfile.DeepCopy()
			newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{
				AccessProviders: []cpv1alpha2.AccessProvider{{
					Name: "kubeconfig",
					Cluster: cpv1alpha2.Cluster{
						Server: "https://cluster.example.com",
						Extensions: []cpv1alpha2.NamedExtension{
							{
								Name: "client.authentication.k8s.io/exec",
								Extension: runtime.RawExtension{
									Raw: []byte(`{"audience":"first"}`),
								},
							},
							{
								Name: "client.authentication.k8s.io/exec",
								Extension: runtime.RawExtension{
									Raw: []byte(`{"audience":"second"}`),
								},
							},
						},
					},
				}},
			}

			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				newClusterProfile,
				metav1.UpdateOptions{},
			)

			gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
		})

		ginkgo.It("Should reject an access provider with an empty name", func(ctx context.Context) {
			newClusterProfile := clusterProfile.DeepCopy()
			newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{
				AccessProviders: []cpv1alpha2.AccessProvider{{
					Name:    "",
					Cluster: cpv1alpha2.Cluster{Server: "https://cluster.example.com"},
				}},
			}
			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				newClusterProfile,
				metav1.UpdateOptions{},
			)
			gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
		})

		ginkgo.DescribeTable("Should reject an access provider whose server is not a URL with a host",
			func(ctx context.Context, server string) {
				newClusterProfile := clusterProfile.DeepCopy()
				newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{
					AccessProviders: []cpv1alpha2.AccessProvider{{
						Name:    "kubeconfig",
						Cluster: cpv1alpha2.Cluster{Server: server},
					}},
				}
				_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
					ctx,
					newClusterProfile,
					metav1.UpdateOptions{},
				)
				gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
			},
			ginkgo.Entry("empty", ""),
			ginkgo.Entry("not a URL", "not-a-url"),
			ginkgo.Entry("no host", "https://"),
			ginkgo.Entry("path without a host", "https:///path"),
		)

		ginkgo.It("Should reject a certificate-authority file path as an unknown field", func(ctx context.Context) {
			dynamicClient, err := dynamic.NewForConfig(cfg)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gvr := cpv1alpha2.ClusterProfileSchemeGroupVersionResource
			current, err := dynamicClient.Resource(gvr).Namespace(testNamespace).Get(
				ctx, clusterName, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			current.Object["status"] = map[string]interface{}{
				"accessProviders": []interface{}{map[string]interface{}{
					"name": "kubeconfig",
					"cluster": map[string]interface{}{
						"server":                "https://cluster.example.com",
						"certificate-authority": "/etc/ca.crt",
					},
				}},
			}
			_, err = dynamicClient.Resource(gvr).Namespace(testNamespace).UpdateStatus(
				ctx,
				current,
				metav1.UpdateOptions{FieldValidation: metav1.FieldValidationStrict},
			)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("certificate-authority"))
		})

		ginkgo.It("Should reject an access provider whose server exceeds 2048 characters", func(ctx context.Context) {
			newClusterProfile := clusterProfile.DeepCopy()
			newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{
				AccessProviders: []cpv1alpha2.AccessProvider{{
					Name: "kubeconfig",
					Cluster: cpv1alpha2.Cluster{
						Server: "https://cluster.example.com/" + strings.Repeat("a", 2048),
					},
				}},
			}
			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				newClusterProfile,
				metav1.UpdateOptions{},
			)
			gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
		})

		ginkgo.It("Should accept 64 access providers", func(ctx context.Context) {
			providers := make([]cpv1alpha2.AccessProvider, 0, 64)
			for i := 0; i < 64; i++ {
				providers = append(providers, cpv1alpha2.AccessProvider{
					Name:    fmt.Sprintf("provider-%d", i),
					Cluster: cpv1alpha2.Cluster{Server: "https://cluster.example.com"},
				})
			}
			newClusterProfile := clusterProfile.DeepCopy()
			newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{AccessProviders: providers}
			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				newClusterProfile,
				metav1.UpdateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should reject 65 access providers", func(ctx context.Context) {
			providers := make([]cpv1alpha2.AccessProvider, 0, 65)
			for i := 0; i < 65; i++ {
				providers = append(providers, cpv1alpha2.AccessProvider{
					Name:    fmt.Sprintf("provider-%d", i),
					Cluster: cpv1alpha2.Cluster{Server: "https://cluster.example.com"},
				})
			}
			newClusterProfile := clusterProfile.DeepCopy()
			newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{AccessProviders: providers}
			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
				ctx,
				newClusterProfile,
				metav1.UpdateOptions{},
			)
			gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
		})

		ginkgo.DescribeTable("Should reject an access provider with an invalid proxy-url",
			func(ctx context.Context, proxyURL string) {
				newClusterProfile := clusterProfile.DeepCopy()
				newClusterProfile.Status = cpv1alpha2.ClusterProfileStatus{
					AccessProviders: []cpv1alpha2.AccessProvider{{
						Name: "kubeconfig",
						Cluster: cpv1alpha2.Cluster{
							Server:   "https://cluster.example.com",
							ProxyURL: proxyURL,
						},
					}},
				}
				_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(testNamespace).UpdateStatus(
					ctx,
					newClusterProfile,
					metav1.UpdateOptions{},
				)
				gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
			},
			ginkgo.Entry("unsupported scheme", "ftp://proxy.example.com"),
			ginkgo.Entry("not a URL", "not-a-url"),
			ginkgo.Entry("no host", "socks5://"),
		)

		ginkgo.It("Should accept an access provider with an explicit empty proxy-url", func(ctx context.Context) {
			dynamicClient, err := dynamic.NewForConfig(cfg)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gvr := cpv1alpha2.ClusterProfileSchemeGroupVersionResource
			current, err := dynamicClient.Resource(gvr).Namespace(testNamespace).Get(
				ctx, clusterName, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			current.Object["status"] = map[string]interface{}{
				"accessProviders": []interface{}{map[string]interface{}{
					"name": "kubeconfig",
					"cluster": map[string]interface{}{
						"server":    "https://cluster.example.com",
						"proxy-url": "",
					},
				}},
			}
			_, err = dynamicClient.Resource(gvr).Namespace(testNamespace).UpdateStatus(
				ctx,
				current,
				metav1.UpdateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

	})
})
