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

package conformance

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

var _ = Describe("Connectivity to remote services", func() {
	ctx := context.TODO()

	// clusters in the system
	var clusters *v1alpha1.ClusterProfileList
	var err error

	BeforeEach(func() {
		Expect(client).ToNot(BeNil())

		clusters, err = client.clusterInventory.ApisV1alpha1().
			ClusterProfiles(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})

		Expect(err).ToNot(HaveOccurred())
	})

	Context("Should alway have cluster manager field set", func() {
		It("should have cluster manager field", Label(RequiredLabel), func() {
			AddReportEntry(
				SpecRefReportEntry,
				"https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/4322-cluster-inventory")
			By("check all cluster profiles", func() {
				for _, cluster := range clusters.Items {
					Expect(len(cluster.Spec.ClusterManager.Name)).ToNot(BeZero(), reportNonConformant(""))
				}
			})
		})

		It("should have cluster manager label set", Label(RequiredLabel), func() {
			AddReportEntry(
				SpecRefReportEntry,
				"https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/4322-cluster-inventory")
			By("check all cluster profiles", func() {
				for _, cluster := range clusters.Items {
					Expect(len(cluster.Labels[v1alpha1.LabelClusterManagerKey])).ToNot(BeZero(), reportNonConformant(""))
				}
			})
		})
	})
})
