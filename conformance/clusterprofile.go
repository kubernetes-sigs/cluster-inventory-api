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
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cpv1alpha2 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
)

// joinedConditionType is the Joined condition predefined by KEP-4322 but not
// (yet) exported as a constant by the API package.
const joinedConditionType = "Joined"

// inventoryProfiles lists every ClusterProfile object in the inventory
// namespace, regardless of which cluster manager created it.
func inventoryProfiles(ctx context.Context) []cpv1alpha2.ClusterProfile {
	list, err := clusterProfileClient.ApisV1alpha2().ClusterProfiles(namespace).List(ctx, metav1.ListOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), fmt.Sprintf(
		"error listing ClusterProfile objects in the inventory namespace %q", namespace))

	return list.Items
}

// profilesUnderTest returns the inventory's ClusterProfile objects scoped to
// the cluster manager under test when --cluster-manager is set.
func profilesUnderTest(ctx context.Context) []cpv1alpha2.ClusterProfile {
	profiles := inventoryProfiles(ctx)
	if clusterManager == "" {
		return profiles
	}

	scoped := []cpv1alpha2.ClusterProfile{}
	for i := range profiles {
		if profiles[i].Spec.ClusterManager.Name == clusterManager {
			scoped = append(scoped, profiles[i])
		}
	}

	return scoped
}

// requireProfilesUnderTest is profilesUnderTest for specs that verify each
// observed object: an empty inventory fails them as undetermined (rather than
// passing vacuously), while the registered-cluster representation spec is the
// one reporting non-conformance for it.
func requireProfilesUnderTest(ctx context.Context) []cpv1alpha2.ClusterProfile {
	profiles := profilesUnderTest(ctx)
	gomega.Expect(profiles).ToNot(gomega.BeEmpty(), fmt.Sprintf(
		"no ClusterProfile objects to verify in the inventory namespace %q%s", namespace, managerSuffix()))

	return profiles
}

func managerSuffix() string {
	if clusterManager == "" {
		return ""
	}

	return fmt.Sprintf(" for cluster manager %q", clusterManager)
}

var _ = ginkgo.Describe("ClusterProfile", func() {
	SpecifyWithSpecRef("The cluster manager must represent each of its registered member clusters "+
		"as a ClusterProfile object in the inventory namespace",
		kep4322Ref("whats-the-relationship-between-the-clusterprofile-api-and-cluster-inventory"),
		ginkgo.Label(RequiredLabel), func(ctx context.Context) {
			gomega.Expect(profilesUnderTest(ctx)).ToNot(gomega.BeEmpty(), reportNonConformant(fmt.Sprintf(
				"no ClusterProfile objects found in the inventory namespace %q%s; at least one cluster must be "+
					"registered with the implementation under test before running the suite", namespace, managerSuffix())))
		})

	SpecifyWithSpecRef(fmt.Sprintf("The cluster manager must add the %s label to each ClusterProfile it creates, "+
		"with the name of the cluster manager as its value", cpv1alpha2.LabelClusterManagerKey),
		kep4322Ref("cluster-manager"),
		ginkgo.Label(RequiredLabel), func(ctx context.Context) {
			for _, profile := range requireProfilesUnderTest(ctx) {
				value, ok := profile.Labels[cpv1alpha2.LabelClusterManagerKey]
				gomega.Expect(ok).To(gomega.BeTrue(), reportNonConformant(fmt.Sprintf(
					"ClusterProfile %q does not carry the %s label", profile.Name, cpv1alpha2.LabelClusterManagerKey)))
				gomega.Expect(value).To(gomega.Equal(profile.Spec.ClusterManager.Name), reportNonConformant(fmt.Sprintf(
					"the %s label of ClusterProfile %q must match spec.clusterManager.name %q; got %q",
					cpv1alpha2.LabelClusterManagerKey, profile.Name, profile.Spec.ClusterManager.Name, value)))
			}
		})

	SpecifyWithSpecRef(fmt.Sprintf("A ClusterProfile carrying the %s label must have a non-empty value for it",
		cpv1alpha2.LabelInventoryMemberIDKey),
		kep4322Ref("uniqueness-of-the-clusterprofile-object"),
		ginkgo.Label(RequiredLabel), func(ctx context.Context) {
			for _, profile := range requireProfilesUnderTest(ctx) {
				value, ok := profile.Labels[cpv1alpha2.LabelInventoryMemberIDKey]
				if !ok {
					continue
				}

				gomega.Expect(value).ToNot(gomega.BeEmpty(), reportNonConformant(fmt.Sprintf(
					"ClusterProfile %q carries the %s label with an empty value",
					profile.Name, cpv1alpha2.LabelInventoryMemberIDKey)))
			}
		})

	SpecifyWithSpecRef(fmt.Sprintf("The cluster manager should add the %s label to each ClusterProfile it creates",
		cpv1alpha2.LabelInventoryMemberIDKey),
		kep4322Ref("uniqueness-of-the-clusterprofile-object"),
		ginkgo.Label(OptionalLabel), func(ctx context.Context) {
			for _, profile := range requireProfilesUnderTest(ctx) {
				gomega.Expect(profile.Labels[cpv1alpha2.LabelInventoryMemberIDKey]).ToNot(gomega.BeEmpty(),
					reportNonConformant(fmt.Sprintf("ClusterProfile %q does not carry the %s label with a non-empty value",
						profile.Name, cpv1alpha2.LabelInventoryMemberIDKey)))
			}
		})

	SpecifyWithSpecRef("Each member cluster should be represented by at most one ClusterProfile object "+
		"within the inventory",
		kep4322Ref("uniqueness-of-the-clusterprofile-object"),
		ginkgo.Label(OptionalLabel), func(ctx context.Context) {
			// Uniqueness is inventory-wide regardless of which cluster manager created the
			// objects, so this spec inspects the whole namespace. The same member ID in
			// other namespaces is a different inventory and no duplicate.
			profilesByMemberID := map[string][]string{}

			for _, profile := range inventoryProfiles(ctx) {
				if id := profile.Labels[cpv1alpha2.LabelInventoryMemberIDKey]; id != "" {
					profilesByMemberID[id] = append(profilesByMemberID[id], profile.Name)
				}
			}

			for id, names := range profilesByMemberID {
				gomega.Expect(names).To(gomega.HaveLen(1), reportNonConformant(fmt.Sprintf(
					"member cluster %q is represented by more than one ClusterProfile in the inventory "+
						"namespace %q: %s", id, namespace, strings.Join(names, ", "))))
			}
		})

	SpecifyWithSpecRef(fmt.Sprintf("The cluster manager should maintain the %s condition on each ClusterProfile "+
		"it manages", cpv1alpha2.ClusterConditionControlPlaneHealthy),
		kep4322Ref("conditions"),
		ginkgo.Label(OptionalLabel), func(ctx context.Context) {
			for _, profile := range requireProfilesUnderTest(ctx) {
				gomega.Expect(meta.FindStatusCondition(profile.Status.Conditions,
					cpv1alpha2.ClusterConditionControlPlaneHealthy)).ToNot(gomega.BeNil(),
					reportNonConformant(fmt.Sprintf("ClusterProfile %q does not report the %s condition",
						profile.Name, cpv1alpha2.ClusterConditionControlPlaneHealthy)))
			}
		})

	SpecifyWithSpecRef(fmt.Sprintf("The cluster manager should report the %s condition on ClusterProfiles "+
		"of clusters under its management", joinedConditionType),
		kep4322Ref("conditions"),
		ginkgo.Label(OptionalLabel), func(ctx context.Context) {
			for _, profile := range requireProfilesUnderTest(ctx) {
				gomega.Expect(meta.FindStatusCondition(profile.Status.Conditions, joinedConditionType)).
					ToNot(gomega.BeNil(), reportNonConformant(fmt.Sprintf(
						"ClusterProfile %q does not report the %s condition", profile.Name, joinedConditionType)))
			}
		})

	SpecifyWithSpecRef("The cluster manager should report the Kubernetes version of the member cluster "+
		"in status.version.kubernetes",
		kep4322Ref("version"),
		ginkgo.Label(OptionalLabel), func(ctx context.Context) {
			for _, profile := range requireProfilesUnderTest(ctx) {
				gomega.Expect(profile.Status.Version.Kubernetes).ToNot(gomega.BeEmpty(), reportNonConformant(
					fmt.Sprintf("ClusterProfile %q does not report the member cluster's Kubernetes version",
						profile.Name)))
			}
		})
})
