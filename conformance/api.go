/*
Copyright The Kubernetes Authors.

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
	"fmt"
	"slices"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cpv1alpha2 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
)

// kep4322 is the specification the conformance specs reference.
const kep4322 = "https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/4322-cluster-inventory"

// kep4322Ref returns a reference to a section of KEP-4322 by its anchor.
func kep4322Ref(anchor string) string {
	return kep4322 + "#" + anchor
}

// serverResources discovers the resources served in the ClusterProfile API
// group version. A group version that is not served is non-conformance; any
// other discovery error leaves conformance undetermined.
func serverResources() *metav1.APIResourceList {
	resources, err := kubernetesClient.Discovery().ServerResourcesForGroupVersion(cpv1alpha2.GroupVersion.String())
	gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeFalse(), reportNonConformant(fmt.Sprintf(
		"the %s API group version is not served by the cluster", cpv1alpha2.GroupVersion)))
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), fmt.Sprintf(
		"error discovering the resources of the %s API group version", cpv1alpha2.GroupVersion))

	return resources
}

var _ = ginkgo.Describe("ClusterProfile API availability", ginkgo.Label(clusterProfileAPI.Name), func() {
	SpecifyWithSpecRef(fmt.Sprintf("The cluster must serve the %s resource in the %s API group version",
		cpv1alpha2.ClusterProfileSchemeGroupVersionResource.Resource, cpv1alpha2.GroupVersion),
		kep4322,
		ginkgo.Label(RequiredLabel), func(ctx context.Context) {
			resources := serverResources()

			gomega.Expect(slices.ContainsFunc(resources.APIResources, func(r metav1.APIResource) bool {
				return r.Name == cpv1alpha2.ClusterProfileSchemeGroupVersionResource.Resource
			})).To(gomega.BeTrue(), reportNonConformant(fmt.Sprintf(
				"the %s API group version does not serve the %s resource",
				cpv1alpha2.GroupVersion, cpv1alpha2.ClusterProfileSchemeGroupVersionResource.Resource)))

			// Discovery has established that the resource is served, so a failing
			// list is an access or environment problem rather than non-conformance.
			_, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(namespace).List(ctx, metav1.ListOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), fmt.Sprintf(
				"error listing ClusterProfile objects in the inventory namespace %q", namespace))
		})

	SpecifyWithSpecRef("The clusterprofiles resource must be namespace scoped",
		kep4322Ref("how-should-we-organize-clusterprofile-objects-on-a-hub-cluster"),
		ginkgo.Label(RequiredLabel), func() {
			resources := serverResources()

			idx := slices.IndexFunc(resources.APIResources, func(r metav1.APIResource) bool {
				return r.Name == cpv1alpha2.ClusterProfileSchemeGroupVersionResource.Resource
			})
			gomega.Expect(idx).ToNot(gomega.Equal(-1), reportNonConformant(fmt.Sprintf(
				"the %s API group version does not serve the %s resource",
				cpv1alpha2.GroupVersion, cpv1alpha2.ClusterProfileSchemeGroupVersionResource.Resource)))

			gomega.Expect(resources.APIResources[idx].Namespaced).To(gomega.BeTrue(), reportNonConformant(
				"the clusterprofiles resource must be namespace scoped"))
		})
})
