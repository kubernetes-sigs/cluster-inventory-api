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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cp "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
)

const (
	OrderedDecisionSlices   = "OrderedDecisionSlices"
	WorkloadScopedDecisions = "WorkloadScopedDecisions"
	SchedulerName           = "SchedulerName"
	DecisionReason          = "DecisionReason"
	DeterministicSliceNames = "DeterministicSliceNames"
)

const placementDecisionKEP = "keps/sig-multicluster/5313-placement-decision-api/README.md"

func placementDecisionRef(anchor string) string {
	return kepRef(PlacementDecisionSpecRevision, placementDecisionKEP, anchor)
}

// placementDecisionCatalog lists what a PlacementDecision producer is checked against.
var placementDecisionCatalog = []Requirement{
	{
		ID:          CheckPlacementDecisionAPIAvailability,
		Description: "PlacementDecision is served in multicluster.x-k8s.io/v1alpha2",
		Level:       LevelMust,
		Scope:       ScopeAPI,
		Ref:         placementDecisionRef("api-specification"),
	},
	{
		ID:          CheckPlacementDecisionNamespaced,
		Description: "PlacementDecision is namespace scoped",
		Level:       LevelMust,
		Scope:       ScopeAPI,
		Ref:         placementDecisionRef("api-specification"),
	},
	{
		ID:          CheckReferenceResolution,
		Description: "Every clusterProfileRef names an existing ClusterProfile",
		Level:       LevelMust,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("relationship-to-other-sig-multicluster-sig-mc-apis"),
	},
	{
		ID:          CheckReferenceUniqueness,
		Description: "A decision lists each ClusterProfile at most once",
		Level:       LevelMust,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("summary"),
	},
	{
		ID:          CheckDecisionKeyCorrelation,
		Description: "Slices of a decision spanning several slices carry a non-empty decision-key",
		Level:       LevelMust,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("slicing"),
	},
	{
		ID:          CheckDecisionIndexOrdering,
		Description: "decision-index numbers the slices of a decision 0, 1, 2, ... without gaps or duplicates",
		Level:       LevelShould,
		Feature:     OrderedDecisionSlices,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("slicing"),
	},
	{
		ID:          CheckPlacementKeyPresence,
		Description: "Slices carry one non-empty placement-key per decision",
		Level:       LevelMay,
		Feature:     WorkloadScopedDecisions,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("slicing"),
	},
	{
		ID:          CheckSchedulerNamePresence,
		Description: "schedulerName is reported",
		Level:       LevelInformative,
		Feature:     SchedulerName,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("api-definition"),
	},
	{
		ID:          CheckDecisionReasonPresence,
		Description: "Every decision entry reports a reason",
		Level:       LevelInformative,
		Feature:     DecisionReason,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("lifecycle"),
	},
	{
		ID:          CheckSliceNameMatchesIndex,
		Description: "Slices of a decision are named <base>-<decision-index>",
		Level:       LevelInformative,
		Feature:     DeterministicSliceNames,
		Scope:       ScopeDecision,
		Ref:         placementDecisionRef("consumer-discovery-and-usage"),
	},
}

var placementDecisionResource = servedResource{
	name:              cp.PlacementDecisionSchemeGroupVersionResource.Resource,
	listing:           "placement decisions",
	availabilityCheck: CheckPlacementDecisionAPIAvailability,
	namespacedCheck:   CheckPlacementDecisionNamespaced,
}

var placementDecisionProducer = profile{
	name:     PlacementDecisionProducerProfile,
	api:      PlacementDecisionAPI,
	spec:     Specification{Revision: PlacementDecisionSpecRevision, URL: placementDecisionRef("")},
	catalog:  placementDecisionCatalog,
	features: featuresOf(placementDecisionCatalog),
	digest:   digestOf(placementDecisionCatalog),
	environment: func(o Options) Environment {
		return Environment{
			DecisionNamespace: o.DecisionNamespace,
			SchedulerName:     o.SchedulerName,
			ObservedDecisions: []ObjectReference{},
		}
	},
	validateEnvironment: func(e Environment) error {
		if e.DecisionNamespace == "" {
			return errors.New("decision-namespace is required: select the namespace a scheduler publishes to")
		}
		return nil
	},
	observed: func(e Environment) (string, string, []ObjectReference) {
		return "decision", e.DecisionNamespace, e.ObservedDecisions
	},
	namespaceColumn: "Decision namespace",
	observe:         observePlacementDecisions,
}

// decisionGroup is what a consumer reads as one decision: the slices sharing a
// decision-key, or a single slice without one.
type decisionGroup struct {
	key    string
	slices []cp.PlacementDecision
}

func (g decisionGroup) name() string {
	if g.key == "" {
		return sliceName(g.slices[0])
	}
	return "decision-key " + strconv.Quote(g.key)
}

type decisionSnapshot struct {
	discovered
	// decisions are the slices published by the scheduler under test, sorted by name.
	decisions []cp.PlacementDecision
	groups    []decisionGroup
	listError error
	// profiles are the ClusterProfiles in every namespace a reference points to.
	profiles      map[types.NamespacedName]bool
	profileErrors map[string]error
}

func observePlacementDecisions(
	ctx context.Context,
	o Options,
	r *ConformanceReport,
) (func(Requirement) TestResult, error) {
	if err := readNamespace(ctx, o, "decision", o.DecisionNamespace); err != nil {
		return nil, err
	}
	var s decisionSnapshot
	s.resources, s.apiError = serverResources(ctx, o.Kubernetes.Discovery())
	s.listError = s.apiError
	if s.apiError == nil {
		var list *cp.PlacementDecisionList
		list, s.listError = o.ClusterInventory.ApisV1alpha2().PlacementDecisions(o.DecisionNamespace).List(
			ctx,
			metav1.ListOptions{},
		)
		if s.listError == nil {
			s.decisions = slices.DeleteFunc(list.Items, func(d cp.PlacementDecision) bool {
				return o.SchedulerName != "" && d.SchedulerName != o.SchedulerName
			})
		}
	}
	slices.SortFunc(s.decisions, func(a, b cp.PlacementDecision) int { return strings.Compare(a.Name, b.Name) })
	s.groups = groupDecisions(s.decisions)
	s.profiles, s.profileErrors = listReferencedProfiles(ctx, o, s.decisions)
	for _, d := range s.decisions {
		r.Environment.ObservedDecisions = append(r.Environment.ObservedDecisions, referenceTo(d.ObjectMeta))
	}
	return func(req Requirement) TestResult { return checkDecisions(req, s) }, nil
}

func groupDecisions(decisions []cp.PlacementDecision) []decisionGroup {
	groups := []decisionGroup{}
	byKey := map[string]int{}
	for _, d := range decisions {
		key := d.Labels[cp.DecisionKeyLabel]
		if key == "" {
			groups = append(groups, decisionGroup{slices: []cp.PlacementDecision{d}})
			continue
		}
		if i, ok := byKey[key]; ok {
			groups[i].slices = append(groups[i].slices, d)
			continue
		}
		byKey[key] = len(groups)
		groups = append(groups, decisionGroup{key: key, slices: []cp.PlacementDecision{d}})
	}
	return groups
}

// listReferencedProfiles lists ClusterProfiles once per referenced namespace,
// which may lie outside the decision namespace.
func listReferencedProfiles(
	ctx context.Context,
	o Options,
	decisions []cp.PlacementDecision,
) (map[types.NamespacedName]bool, map[string]error) {
	namespaces := []string{}
	for _, d := range decisions {
		for _, e := range d.Decisions {
			namespaces = append(namespaces, profileRef(d, e).Namespace)
		}
	}
	slices.Sort(namespaces)
	profiles, errs := map[types.NamespacedName]bool{}, map[string]error{}
	for _, namespace := range slices.Compact(namespaces) {
		list, err := o.ClusterInventory.ApisV1alpha2().ClusterProfiles(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errs[namespace] = err
			continue
		}
		for _, p := range list.Items {
			profiles[types.NamespacedName{Namespace: p.Namespace, Name: p.Name}] = true
		}
	}
	return profiles, errs
}

// profileRef resolves a reference; an empty namespace means the decision's own.
func profileRef(d cp.PlacementDecision, e cp.ClusterDecision) types.NamespacedName {
	namespace := e.ClusterProfileRef.Namespace
	if namespace == "" {
		namespace = d.Namespace
	}
	return types.NamespacedName{Namespace: namespace, Name: e.ClusterProfileRef.Name}
}

func sliceName(d cp.PlacementDecision) string {
	return types.NamespacedName{Namespace: d.Namespace, Name: d.Name}.String()
}

// decisionIndex reads a slice's decision-index. valid requires a plain
// non-negative decimal, the form consumers can sort by.
func decisionIndex(d cp.PlacementDecision) (index int, present, valid bool) {
	value, present := d.Labels[cp.DecisionIndexLabel]
	if !present {
		return 0, false, false
	}
	index, err := strconv.Atoi(value)
	return index, true, err == nil && index >= 0 && strconv.Itoa(index) == value
}

func checkDecisions(req Requirement, s decisionSnapshot) TestResult {
	if req.Scope == ScopeAPI {
		return checkAPI(req, s.discovered, placementDecisionResource, s.listError)
	}
	if s.listError != nil {
		return outcome(req, Unknown, "Cannot list placement decisions: "+s.listError.Error())
	}
	if len(s.decisions) == 0 {
		return outcome(req, Unknown, "No PlacementDecision to verify; publish at least one decision before running")
	}
	switch req.ID {
	case CheckReferenceResolution:
		return checkReferences(req, s)
	case CheckReferenceUniqueness:
		return checkUniqueReferences(req, s.groups)
	case CheckDecisionKeyCorrelation:
		return checkDecisionKeys(req, s.decisions)
	case CheckDecisionIndexOrdering:
		return checkIndexOrdering(req, s.groups)
	case CheckPlacementKeyPresence:
		return checkPlacementKeys(req, s.groups)
	case CheckSliceNameMatchesIndex:
		return checkSliceNames(req, s.groups)
	}
	return checkReported(req, s.decisions)
}

func checkReferences(req Requirement, s decisionSnapshot) TestResult {
	missing := []string{}
	unlisted := map[string]error{}
	for _, d := range s.decisions {
		for _, e := range d.Decisions {
			ref := profileRef(d, e)
			if err, ok := s.profileErrors[ref.Namespace]; ok {
				unlisted[ref.Namespace] = err
			} else if !s.profiles[ref] {
				missing = append(missing, sliceName(d)+" references "+ref.String())
			}
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return outcome(req, Failed, "No such ClusterProfile: "+strings.Join(slices.Compact(missing), ", "))
	}
	if len(unlisted) > 0 {
		reasons := []string{}
		for _, namespace := range slices.Sorted(maps.Keys(unlisted)) {
			reasons = append(reasons, namespace+": "+unlisted[namespace].Error())
		}
		return outcome(req, Unknown, "Cannot list ClusterProfiles in "+strings.Join(reasons, "; "))
	}
	return outcome(req, Passed, "")
}

func checkUniqueReferences(req Requirement, groups []decisionGroup) TestResult {
	duplicates := []string{}
	for _, g := range groups {
		listedIn := map[types.NamespacedName]string{}
		for _, d := range g.slices {
			for _, e := range d.Decisions {
				ref := profileRef(d, e)
				if first, ok := listedIn[ref]; ok {
					duplicates = append(
						duplicates,
						fmt.Sprintf("%s lists %s more than once (%s, %s)", g.name(), ref, first, sliceName(d)),
					)
					continue
				}
				listedIn[ref] = sliceName(d)
			}
		}
	}
	if len(duplicates) > 0 {
		return outcome(req, Failed, strings.Join(duplicates, "; "))
	}
	return outcome(req, Passed, "")
}

// checkDecisionKeys can only see a multi-slice decision through its labels: a
// decision-index above 0 means slice 0 exists too, so the key is required. An
// empty key groups nothing, so it counts as missing.
func checkDecisionKeys(req Requirement, decisions []cp.PlacementDecision) TestResult {
	violations := []string{}
	for _, d := range decisions {
		if index, _, valid := decisionIndex(d); valid && index > 0 && d.Labels[cp.DecisionKeyLabel] == "" {
			violations = append(violations, fmt.Sprintf("%s: decision-index %d has no decision-key", sliceName(d), index))
		}
	}
	if len(violations) > 0 {
		return outcome(req, Failed, strings.Join(violations, "; "))
	}
	return outcome(req, Passed, "")
}

func checkIndexOrdering(req Requirement, groups []decisionGroup) TestResult {
	problems := []string{}
	ordered := 0
	for _, g := range groups {
		issues := []string{}
		bySlice := map[int][]string{}
		uses := false
		for _, d := range g.slices {
			index, present, valid := decisionIndex(d)
			uses = uses || present
			switch {
			case !present:
				issues = append(issues, sliceName(d)+" has no decision-index")
			case !valid:
				issues = append(issues, fmt.Sprintf("%s has decision-index %q, not a non-negative integer",
					sliceName(d), d.Labels[cp.DecisionIndexLabel]))
			default:
				bySlice[index] = append(bySlice[index], sliceName(d))
			}
		}
		if !uses {
			continue
		}
		ordered++
		indexes := slices.Sorted(maps.Keys(bySlice))
		for _, index := range indexes {
			if len(bySlice[index]) > 1 {
				issues = append(issues, fmt.Sprintf("decision-index %d is on %s", index, strings.Join(bySlice[index], ", ")))
			}
		}
		for want, index := range indexes {
			if index == want {
				continue
			}
			if want == 0 {
				issues = append(issues,
					"decision-index does not start at 0; slice 0 is missing or carries a different decision-key")
			} else {
				issues = append(issues, fmt.Sprintf("decision-index %d is missing", want))
			}
			break
		}
		for _, issue := range issues {
			problems = append(problems, g.name()+": "+issue)
		}
	}
	if ordered == 0 {
		return outcome(req, Skipped, "No decision uses decision-index")
	}
	if len(problems) > 0 {
		// A snapshot taken during a reschedule can also see a partial update.
		return outcome(req, Failed, "Inconsistent when observed: "+strings.Join(problems, "; "))
	}
	return outcome(req, Passed, "")
}

func checkPlacementKeys(req Requirement, groups []decisionGroup) TestResult {
	missing, mixed := []string{}, []string{}
	for _, g := range groups {
		values := map[string]bool{}
		for _, d := range g.slices {
			if key := d.Labels[cp.PlacementKeyLabel]; key != "" {
				values[key] = true
			} else {
				missing = append(missing, sliceName(d))
			}
		}
		if len(values) > 1 {
			mixed = append(mixed, fmt.Sprintf("%s has placement-key values %s",
				g.name(), strings.Join(slices.Sorted(maps.Keys(values)), ", ")))
		}
	}
	if len(mixed) > 0 {
		return outcome(req, Failed, strings.Join(mixed, "; "))
	}
	return unmet(req, "Not reported on: ", missing)
}

func checkSliceNames(req Requirement, groups []decisionGroup) TestResult {
	mismatched := []string{}
	compared := 0
	for _, g := range groups {
		base := ""
		for _, d := range g.slices {
			index, _, valid := decisionIndex(d)
			if !valid {
				continue
			}
			compared++
			cut := strings.LastIndex(d.Name, "-")
			if cut < 0 || d.Name[cut+1:] != strconv.Itoa(index) || base != "" && d.Name[:cut] != base {
				mismatched = append(mismatched, fmt.Sprintf("%s (decision-index %d)", sliceName(d), index))
				continue
			}
			base = d.Name[:cut]
		}
	}
	if compared == 0 {
		return outcome(req, Skipped, "No decision uses decision-index")
	}
	return unmet(req, "Not named <base>-<decision-index>: ", mismatched)
}

func checkReported(req Requirement, decisions []cp.PlacementDecision) TestResult {
	missing := []string{}
	for _, d := range decisions {
		switch req.ID {
		case CheckSchedulerNamePresence:
			if d.SchedulerName == "" {
				missing = append(missing, sliceName(d))
			}
		case CheckDecisionReasonPresence:
			if slices.ContainsFunc(d.Decisions, func(e cp.ClusterDecision) bool { return e.Reason == "" }) {
				missing = append(missing, sliceName(d))
			}
		}
	}
	return unmet(req, "Not reported on: ", missing)
}
