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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

const (
	ClusterManagerLabel       = "ClusterManagerLabel"
	InventoryMemberID         = "InventoryMemberID"
	InventoryMemberUniqueness = "InventoryMemberUniqueness"
	ControlPlaneHealthy       = "ControlPlaneHealthy"
	Joined                    = "Joined"
	KubernetesVersion         = "KubernetesVersion"
)

var features = []string{
	ClusterManagerLabel,
	InventoryMemberID,
	InventoryMemberUniqueness,
	ControlPlaneHealthy,
	Joined,
	KubernetesVersion,
}

func SupportedFeatureNames() []string { return slices.Clone(features) }

func specRef(anchor string) string {
	base := "https://github.com/kubernetes/enhancements/blob/" + SpecRevision +
		"/keps/sig-multicluster/4322-cluster-inventory/README.md"
	if anchor != "" {
		base += "#" + anchor
	}
	return base
}

// Catalog returns a copy so callers cannot change the acceptance criteria.
func Catalog() []Requirement { return slices.Clone(catalog) }

var catalog = []Requirement{
	{
		ID:          CheckAPIAvailability,
		Description: "ClusterProfile is served in multicluster.x-k8s.io/v1alpha2",
		Level:       LevelMust,
		Scope:       ScopeAPI,
		Ref:         specRef(""),
	},
	{
		ID:          CheckNamespaced,
		Description: "ClusterProfile is namespace scoped",
		Level:       LevelMust,
		Scope:       ScopeAPI,
		Ref:         specRef("how-should-we-organize-clusterprofile-objects-on-a-hub-cluster"),
	},
	{
		ID:          CheckRepresentation,
		Description: "Registered clusters are represented in the selected inventory",
		Level:       LevelMust,
		Scope:       ScopeManager,
		Ref:         specRef("whats-the-relationship-between-the-clusterprofile-api-and-cluster-inventory"),
	},
	{
		ID:          CheckManagerValue,
		Description: "A cluster manager label, when present, equals spec.clusterManager.name",
		Level:       LevelMust,
		Scope:       ScopeManager,
		Ref:         specRef("cluster-manager"),
	},
	{
		ID:          CheckMemberIDValue,
		Description: "An inventory member ID, when present, is non-empty",
		Level:       LevelMust,
		Scope:       ScopeManager,
		Ref:         specRef("uniqueness-of-the-clusterprofile-object"),
	},
	{
		ID:          CheckManagerPresence,
		Description: "Cluster manager labels are provided",
		Level:       "MAY",
		Feature:     ClusterManagerLabel,
		Scope:       ScopeManager,
		Ref:         specRef("cluster-manager"),
	},
	{
		ID:          CheckMemberIDPresence,
		Description: "Inventory member IDs are provided",
		Level:       LevelShould,
		Feature:     InventoryMemberID,
		Scope:       ScopeManager,
		Ref:         specRef("uniqueness-of-the-clusterprofile-object"),
	},
	{
		ID:          InventoryMemberUniqueness,
		Description: "Each member ID occurs at most once within an inventory",
		Level:       LevelShould,
		Feature:     InventoryMemberUniqueness,
		Scope:       ScopeInventory,
		Ref:         specRef("uniqueness-of-the-clusterprofile-object"),
	},
	{
		ID:          CheckHealthyPresence,
		Description: "ControlPlaneHealthy is reported",
		Level:       "Informative",
		Feature:     ControlPlaneHealthy,
		Scope:       ScopeManager,
		Ref:         specRef("conditions"),
	},
	{
		ID:          CheckJoinedPresence,
		Description: "Joined is reported",
		Level:       LevelShould,
		Feature:     Joined,
		Scope:       ScopeManager,
		Ref:         specRef("conditions"),
	},
	{
		ID:          CheckVersionPresence,
		Description: "Kubernetes version is reported",
		Level:       "Informative",
		Feature:     KubernetesVersion,
		Scope:       ScopeManager,
		Ref:         specRef("version"),
	},
}

func CatalogDigest() string {
	data, _ := json.Marshal(catalog)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func gate(t TestResult, supported []string) bool {
	return t.Feature == "" || slices.Contains(supported, t.Feature)
}

func summarize(tests []TestResult, supported []string) (Statistics, Result) {
	stats := Statistics{Total: len(tests)}
	result := Success
	gated := 0
	for _, t := range tests {
		switch t.Outcome {
		case Passed:
			stats.Passed++
		case Failed:
			stats.Failed++
		case Skipped:
			stats.Skipped++
		case Unknown:
			stats.Unknown++
		case NotSupported:
			stats.NotSupported++
		}
		if !gate(t, supported) {
			continue
		}
		gated++
		switch t.Outcome {
		case Failed, NotSupported:
			result = combine(result, Failure)
		case Unknown:
			result = combine(result, Undetermined)
		case Skipped:
			result = combine(result, Partial)
		}
	}
	if gated == 0 {
		result = combine(result, Undetermined)
	}
	return stats, result
}

func combine(a, b Result) Result {
	rank := map[Result]int{Success: 0, Partial: 1, Undetermined: 2, Failure: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
