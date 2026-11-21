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
	"sync"
)

const (
	ClusterManagerLabel       = "ClusterManagerLabel"
	InventoryMemberID         = "InventoryMemberID"
	InventoryMemberUniqueness = "InventoryMemberUniqueness"
	ControlPlaneHealthy       = "ControlPlaneHealthy"
	Joined                    = "Joined"
	KubernetesVersion         = "KubernetesVersion"
)

// features lists each optional ClusterProfile capability once, in catalog order.
var features = featuresOf(catalog)

// featuresOf lists each optional capability of a catalog once, in catalog order.
func featuresOf(c []Requirement) []string {
	out := []string{}
	for _, req := range c {
		if req.Optional() && !slices.Contains(out, req.Feature) {
			out = append(out, req.Feature)
		}
	}
	return out
}

func SupportedFeatureNames() []string { return slices.Clone(features) }

const clusterInventoryKEP = "keps/sig-multicluster/4322-cluster-inventory/README.md"

func specRef(anchor string) string { return kepRef(SpecRevision, clusterInventoryKEP, anchor) }

// kepRef links a section of a KEP at the pinned revision the catalog was written against.
func kepRef(revision, path, anchor string) string {
	base := "https://github.com/kubernetes/enhancements/blob/" + revision + "/" + path
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
		Level:       LevelMay,
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
		Level:       LevelInformative,
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
		Level:       LevelInformative,
		Feature:     KubernetesVersion,
		Scope:       ScopeManager,
		Ref:         specRef("version"),
	},
}

var catalogDigest = digestOf(catalog)

// digestOf fingerprints a catalog so a report can only be validated by the suite that wrote it.
func digestOf(c []Requirement) func() string {
	return sync.OnceValue(func() string {
		data, _ := json.Marshal(c)
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	})
}

func CatalogDigest() string { return catalogDigest() }

var clusterProfileProducer = profile{
	name:     ProducerProfile,
	api:      ClusterProfileAPI,
	spec:     Specification{Revision: SpecRevision, URL: specRef("")},
	catalog:  catalog,
	features: features,
	digest:   catalogDigest,
	environment: func(o Options) Environment {
		return Environment{
			InventoryNamespace: o.Namespace,
			ClusterManager:     o.ClusterManager,
			ExpectedClusters:   o.ExpectedClusters,
			ObservedProfiles:   []ObjectReference{},
		}
	},
	validateEnvironment: func(e Environment) error { return validateInventory(e.InventoryNamespace, e.ExpectedClusters) },
	observed: func(e Environment) (string, string, []ObjectReference) {
		return "profile", e.InventoryNamespace, e.ObservedProfiles
	},
	namespaceColumn: "Inventory",
	observe:         observeClusterProfiles,
}

// registry lists every profile. The first entry is the default selection, so
// runs that select no API or profile keep observing ClusterProfiles only.
var registry = []profile{clusterProfileProducer, placementDecisionProducer}

func lookupProfile(name string) (profile, bool) {
	idx := slices.IndexFunc(registry, func(p profile) bool { return p.name == name })
	if idx < 0 {
		return profile{}, false
	}
	return registry[idx], true
}

func gate(t TestResult, supported []string) bool {
	return !t.Optional() || slices.Contains(supported, t.Feature)
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

var resultRank = map[Result]int{Success: 0, Partial: 1, Undetermined: 2, Failure: 3}

func combine(a, b Result) Result {
	if resultRank[b] > resultRank[a] {
		return b
	}
	return a
}

// overall rolls profile results and run errors up into the report result.
func overall(r *ConformanceReport) Result {
	result := Success
	if len(r.Profiles) == 0 {
		result = Undetermined
	}
	for _, p := range r.Profiles {
		result = combine(result, p.Result)
	}
	if len(r.Errors) > 0 {
		result = combine(result, Undetermined)
	}
	return result
}
