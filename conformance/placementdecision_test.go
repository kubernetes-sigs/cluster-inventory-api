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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	cp "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
	cpfake "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned/fake"
)

const (
	testDecisions = "decisions"
	testScheduler = "scheduler"
)

func sampleDecision(name, scheduler string) *cp.PlacementDecision {
	return &cp.PlacementDecision{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       testDecisions,
			UID:             types.UID(name + "-uid"),
			ResourceVersion: "1",
		},
		Decisions: []cp.ClusterDecision{
			{ClusterProfileRef: cp.ClusterProfileReference{Name: "a", Namespace: testInventory}},
		},
		SchedulerName: scheduler,
	}
}

// decisionSlice is one slice of a decision that meets every check: it is keyed,
// indexed, named after its index, workload scoped, and explains each choice.
func decisionSlice(index int, profiles ...string) *cp.PlacementDecision {
	d := sampleDecision("app-"+strconv.Itoa(index), testScheduler)
	d.Labels = map[string]string{
		cp.DecisionKeyLabel:   "app",
		cp.DecisionIndexLabel: strconv.Itoa(index),
		cp.PlacementKeyLabel:  "app",
	}
	d.Decisions = nil
	for _, name := range profiles {
		d.Decisions = append(d.Decisions, cp.ClusterDecision{
			ClusterProfileRef: cp.ClusterProfileReference{Name: name, Namespace: testInventory},
			Reason:            "selected",
		})
	}
	return d
}

// decisionOptions serves both APIs and seeds two ClusterProfiles plus the given
// decisions, so the same options can select either profile or both.
func decisionOptions(
	t *testing.T,
	decisions ...runtime.Object,
) (Options, *kubefake.Clientset, *cpfake.Clientset) {
	t.Helper()
	profiles := []runtime.Object{sampleProfile("a", testInventory, "id-a"), sampleProfile("b", testInventory, "id-b")}
	o, k, c := sampleOptions(append(profiles, decisions...)...)
	if err := k.Tracker().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testDecisions}}); err != nil {
		t.Fatal(err)
	}
	k.Resources[0].APIResources = append(k.Resources[0].APIResources, metav1.APIResource{
		Name:       cp.PlacementDecisionSchemeGroupVersionResource.Resource,
		Kind:       cp.PlacementDecisionKind,
		Namespaced: true,
	})
	o.DecisionNamespace = testDecisions
	o.SchedulerName = testScheduler
	return o, k, c
}

func TestUnitDefaultSelectionIsClusterProfile(t *testing.T) {
	o, _, _ := decisionOptions(t, sampleDecision("d", testScheduler))
	reports, err := RunAll(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Profiles[0].Name != ProducerProfile {
		t.Fatalf("default selection changed: %+v", reports)
	}
	if len(reports[0].Environment.ObservedDecisions) != 0 || reports[0].Environment.DecisionNamespace != "" {
		t.Fatalf("ClusterProfile report carries PlacementDecision inputs: %+v", reports[0].Environment)
	}
}

func TestUnitPlacementDecisionRun(t *testing.T) {
	o, k, c := decisionOptions(t, decisionSlice(1, "b"), decisionSlice(0, "a"), sampleDecision("other", testOther))
	o.APIs = []string{PlacementDecisionAPI}
	o.SupportedFeatures = featuresOf(placementDecisionCatalog)
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	p := r.Profiles[0]
	if p.Name != PlacementDecisionProducerProfile || p.Statistics.Passed != len(placementDecisionCatalog) {
		t.Fatalf("unexpected report: %+v", r)
	}
	if r.Specification.Revision != PlacementDecisionSpecRevision || r.Suite.CatalogDigest == CatalogDigest() {
		t.Fatalf("report is pinned to the ClusterProfile specification: %+v", r.Specification)
	}
	names := make([]string, 0, len(r.Environment.ObservedDecisions))
	for _, d := range r.Environment.ObservedDecisions {
		names = append(names, d.Name)
	}
	if !slices.Equal(names, []string{"app-0", "app-1"}) {
		t.Fatalf("observed decisions %v, want the scheduler's slices sorted by name", names)
	}
	if err = ValidateReport(r, true); err != nil {
		t.Fatal(err)
	}
	for _, actions := range [][]clienttesting.Action{k.Actions(), c.Actions()} {
		for _, action := range actions {
			if action.GetVerb() != "get" && action.GetVerb() != "list" {
				t.Fatalf("observer wrote resources: %+v", action)
			}
		}
	}

	r.Environment.ObservedDecisions = nil
	if err = ValidateReport(r, true); err == nil {
		t.Fatal("submission without observed decisions was accepted")
	}
}

func TestUnitPlacementDecisionSetup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Options, *kubefake.Clientset)
		id     string
		out    Outcome
	}{
		{"missing decision namespace", func(o *Options, k *kubefake.Clientset) {
			o.DecisionNamespace = ""
		}, CheckPlacementDecisionAPIAvailability, Unknown},
		{"ClusterProfile feature claimed", func(o *Options, k *kubefake.Clientset) {
			o.SupportedFeatures = []string{Joined}
		}, CheckPlacementDecisionAPIAvailability, Unknown},
		{"API not served", func(o *Options, k *kubefake.Clientset) {
			k.Resources[0].APIResources = k.Resources[0].APIResources[:1]
		}, CheckPlacementDecisionAPIAvailability, Failed},
		{"cluster scoped API", func(o *Options, k *kubefake.Clientset) {
			k.Resources[0].APIResources[1].Namespaced = false
		}, CheckPlacementDecisionNamespaced, Failed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, k, _ := decisionOptions(t, sampleDecision("d", testScheduler))
			o.APIs = []string{PlacementDecisionAPI}
			o.SupportedFeatures = nil
			tc.change(&o, k)
			r, err := Run(context.Background(), o)
			if err == nil || findResult(t, r, tc.id).Outcome != tc.out {
				t.Fatalf("err=%v check=%+v", err, findResult(t, r, tc.id))
			}
		})
	}

	o, _, _ := decisionOptions(t, sampleDecision("d", testScheduler))
	o.APIs = []string{PlacementDecisionAPI}
	o.SupportedFeatures = []string{Joined}
	if _, err := Run(context.Background(), o); err == nil ||
		!strings.Contains(err.Error(), `"Joined" is not offered by the selected profiles`) {
		t.Fatalf("feature of an unselected profile: %v", err)
	}
}

func TestUnitBothProfilesWritePerAPIReports(t *testing.T) {
	o, _, _ := decisionOptions(t, sampleDecision("d", testScheduler))
	o.APIs = []string{ClusterProfileAPI, PlacementDecisionAPI}
	if _, err := RunAll(context.Background(), o); err == nil ||
		!strings.Contains(err.Error(), "per-api") {
		t.Fatalf("several profiles were written to one report: %v", err)
	}
	if _, err := Run(context.Background(), o); err == nil {
		t.Fatal("Run observed more than one profile")
	}

	o.ReportPerAPI = true
	reports, err := RunAll(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 {
		t.Fatalf("got %d reports, want one per profile", len(reports))
	}
	if got := reports[1].Profiles[0].SupportedFeatures; len(got) != 0 {
		t.Fatalf("ClusterProfile features leaked into the PlacementDecision report: %v", got)
	}
	dir := t.TempDir()
	for _, r := range reports {
		if err = WriteReports(dir, []string{ReportFormatYAML}, true, r); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"report-clusterprofile.yaml", "report-placementdecision.yaml"} {
		r, err := ReadReport(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateReport(r, true); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("report directory has %d files: %v", len(entries), err)
	}
}

func TestUnitReportValidatedAgainstItsOwnProfile(t *testing.T) {
	o, _, _ := decisionOptions(t, sampleDecision("d", testScheduler))
	o.APIs = []string{PlacementDecisionAPI}
	o.SupportedFeatures = nil
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*ConformanceReport)
	}{
		{"ClusterProfile specification", func(r *ConformanceReport) { r.Specification = clusterProfileProducer.spec }},
		{"ClusterProfile catalog", func(r *ConformanceReport) { r.Suite.CatalogDigest = CatalogDigest() }},
		{"API of another profile", func(r *ConformanceReport) { r.Profiles[0].API = ClusterProfileAPI }},
		{"unknown profile", func(r *ConformanceReport) { r.Profiles[0].Name = "typo" }},
		{"ClusterProfile feature", func(r *ConformanceReport) { r.Profiles[0].SupportedFeatures = []string{Joined} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := r
			changed.Profiles = slices.Clone(r.Profiles)
			tc.change(&changed)
			if err := ValidateReport(changed, false); err == nil {
				t.Fatal("report accepted")
			}
		})
	}
}

func TestUnitPlacementDecisionChecks(t *testing.T) {
	unkeyed := func(d *cp.PlacementDecision) *cp.PlacementDecision {
		delete(d.Labels, cp.DecisionKeyLabel)
		return d
	}
	labeled := func(d *cp.PlacementDecision, key, value string) *cp.PlacementDecision {
		d.Labels[key] = value
		return d
	}
	renamed := func(d *cp.PlacementDecision, name string) *cp.PlacementDecision {
		d.Name = name
		return d
	}
	for _, tc := range []struct {
		name      string
		decisions []runtime.Object
		change    func(*Options, *cpfake.Clientset)
		id        string
		out       Outcome
		message   string
	}{
		{
			name:      "unknown ClusterProfile",
			decisions: []runtime.Object{decisionSlice(0, "missing")},
			id:        CheckReferenceResolution, out: Failed, message: "references inventory/missing",
		},
		{
			name: "reference in the decision's own namespace",
			decisions: []runtime.Object{func() runtime.Object {
				d := decisionSlice(0, "a")
				d.Decisions[0].ClusterProfileRef.Namespace = ""
				return d
			}()},
			id: CheckReferenceResolution, out: Failed, message: "references decisions/a",
		},
		{
			name:      "ClusterProfiles cannot be listed",
			decisions: []runtime.Object{decisionSlice(0, "a")},
			change: func(o *Options, c *cpfake.Clientset) {
				c.PrependReactor("list", testProfileResource, func(clienttesting.Action) (bool, runtime.Object, error) {
					resource := cp.ClusterProfileSchemeGroupVersionResource.GroupResource()
					return true, nil, apierrors.NewForbidden(resource, "", errors.New("denied"))
				})
			},
			id: CheckReferenceResolution, out: Unknown, message: "Cannot list ClusterProfiles in inventory",
		},
		{
			name:      "ClusterProfile listed in two slices",
			decisions: []runtime.Object{decisionSlice(0, "a"), decisionSlice(1, "a")},
			id:        CheckReferenceUniqueness, out: Failed, message: "lists inventory/a more than once",
		},
		{
			name:      "ClusterProfile listed twice in one slice",
			decisions: []runtime.Object{decisionSlice(0, "a", "a")},
			id:        CheckReferenceUniqueness, out: Failed, message: "lists inventory/a more than once",
		},
		{
			name:      "later slice without a decision-key",
			decisions: []runtime.Object{decisionSlice(0, "a"), unkeyed(decisionSlice(1, "b"))},
			id:        CheckDecisionKeyCorrelation, out: Failed, message: "decision-index 1 has no decision-key",
		},
		{
			name:      "empty decision-key on a later slice",
			decisions: []runtime.Object{decisionSlice(0, "a"), labeled(decisionSlice(1, "b"), cp.DecisionKeyLabel, "")},
			id:        CheckDecisionKeyCorrelation, out: Failed, message: "decision-index 1 has no decision-key",
		},
		{
			name:      "empty decision-key on a single slice",
			decisions: []runtime.Object{labeled(decisionSlice(0, "a"), cp.DecisionKeyLabel, "")},
			id:        CheckDecisionKeyCorrelation, out: Passed,
		},
		{
			name:      "single slice without labels",
			decisions: []runtime.Object{sampleDecision("d", testScheduler)},
			id:        CheckDecisionKeyCorrelation, out: Passed,
		},
		{
			name:      "duplicate decision-index",
			decisions: []runtime.Object{decisionSlice(0, "a"), labeled(decisionSlice(1, "b"), cp.DecisionIndexLabel, "0")},
			id:        CheckDecisionIndexOrdering, out: Failed, message: "decision-index 0 is on",
		},
		{
			name:      "decision-index does not start at 0",
			decisions: []runtime.Object{decisionSlice(1, "b")},
			id:        CheckDecisionIndexOrdering, out: Failed, message: "slice 0 is missing",
		},
		{
			name:      "gap in decision-index",
			decisions: []runtime.Object{decisionSlice(0, "a"), decisionSlice(2, "b")},
			id:        CheckDecisionIndexOrdering, out: Failed, message: "decision-index 1 is missing",
		},
		{
			name: "slice without decision-index in an ordered decision",
			decisions: []runtime.Object{decisionSlice(0, "a"), func() runtime.Object {
				d := decisionSlice(1, "b")
				delete(d.Labels, cp.DecisionIndexLabel)
				return d
			}()},
			id: CheckDecisionIndexOrdering, out: Failed, message: "has no decision-index",
		},
		{
			name:      "decision-index that consumers cannot sort",
			decisions: []runtime.Object{labeled(decisionSlice(0, "a"), cp.DecisionIndexLabel, "01")},
			id:        CheckDecisionIndexOrdering, out: Failed, message: "not a non-negative integer",
		},
		{
			name:      "no decision-index",
			decisions: []runtime.Object{sampleDecision("d", testScheduler)},
			id:        CheckDecisionIndexOrdering, out: Skipped,
		},
		{
			name:      "placement-key differs across slices",
			decisions: []runtime.Object{decisionSlice(0, "a"), labeled(decisionSlice(1, "b"), cp.PlacementKeyLabel, "other")},
			id:        CheckPlacementKeyPresence, out: Failed, message: "placement-key values app, other",
		},
		{
			name:      "no placement-key",
			decisions: []runtime.Object{sampleDecision("d", testScheduler)},
			id:        CheckPlacementKeyPresence, out: NotSupported, message: "decisions/d",
		},
		{
			name:      "no schedulerName",
			decisions: []runtime.Object{sampleDecision("d", "")},
			change:    func(o *Options, c *cpfake.Clientset) { o.SchedulerName = "" },
			id:        CheckSchedulerNamePresence, out: NotSupported, message: "decisions/d",
		},
		{
			name:      "decision without a reason",
			decisions: []runtime.Object{sampleDecision("d", testScheduler)},
			id:        CheckDecisionReasonPresence, out: NotSupported, message: "decisions/d",
		},
		{
			name:      "name does not match decision-index",
			decisions: []runtime.Object{renamed(decisionSlice(0, "a"), "app-decision-1")},
			id:        CheckSliceNameMatchesIndex, out: NotSupported, message: "decisions/app-decision-1 (decision-index 0)",
		},
		{
			name:      "slices with different name bases",
			decisions: []runtime.Object{decisionSlice(0, "a"), renamed(decisionSlice(1, "b"), "other-1")},
			id:        CheckSliceNameMatchesIndex, out: NotSupported, message: "decisions/other-1",
		},
		{
			name:      "nothing published",
			decisions: nil,
			id:        CheckReferenceResolution, out: Unknown, message: "No PlacementDecision to verify",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, _, c := decisionOptions(t, tc.decisions...)
			o.APIs = []string{PlacementDecisionAPI}
			o.SupportedFeatures = featuresOf(placementDecisionCatalog)
			o.RunMode = Development
			if tc.change != nil {
				tc.change(&o, c)
			}
			r, _ := Run(context.Background(), o)
			got := findResult(t, r, tc.id)
			if got.Outcome != tc.out || !strings.Contains(got.Message, tc.message) {
				t.Fatalf("got %s %q, want %s containing %q", got.Outcome, got.Message, tc.out, tc.message)
			}
		})
	}
}

// OCM mirrors each of its PlacementDecisions: decision-key and placement-key are
// the Placement name, decision-index is the decision group index, and names
// count from 1. A group of more than 100 clusters spans two slices of index 0.
func TestUnitOCMShapedDecisions(t *testing.T) {
	ocm := func(name string, profiles ...string) *cp.PlacementDecision {
		d := decisionSlice(0, profiles...)
		d.Name = name
		d.Decisions[0].Reason = ""
		return d
	}
	o, _, _ := decisionOptions(t, ocm("app-decision-1", "a"), ocm("app-decision-2", "b"))
	o.APIs = []string{PlacementDecisionAPI}
	o.SupportedFeatures = []string{WorkloadScopedDecisions, SchedulerName}
	o.RunMode = Development
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("unclaimed capabilities failed the run: %v", err)
	}
	for id, want := range map[string]Outcome{
		CheckReferenceResolution:    Passed,
		CheckReferenceUniqueness:    Passed,
		CheckDecisionKeyCorrelation: Passed,
		CheckDecisionIndexOrdering:  Failed,
		CheckDecisionReasonPresence: NotSupported,
		CheckSliceNameMatchesIndex:  NotSupported,
	} {
		if got := findResult(t, r, id).Outcome; got != want {
			t.Errorf("%s: got %s, want %s", id, got, want)
		}
	}
}
