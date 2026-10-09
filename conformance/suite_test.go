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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	cp "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
	cpclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	cpfake "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned/fake"
)

const (
	testManager         = "manager"
	testInventory       = "inventory"
	testOther           = "other"
	testVersion         = "v1.35.0"
	testVersionPath     = "/version"
	testProfileResource = "clusterprofiles"
)

func sampleProfile(name, namespace, id string) *cp.ClusterProfile {
	return &cp.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			UID:             types.UID(name + "-uid"),
			ResourceVersion: "1",
			Labels: map[string]string{
				cp.LabelClusterManagerKey:    testManager,
				cp.LabelInventoryMemberIDKey: id,
			},
		},
		Spec: cp.ClusterProfileSpec{ClusterManager: cp.ClusterManager{Name: testManager}},
		Status: cp.ClusterProfileStatus{
			Version: cp.ClusterVersion{Kubernetes: testVersion},
			Conditions: []metav1.Condition{
				{Type: "ControlPlaneHealthy", Status: metav1.ConditionTrue},
				{Type: "Joined", Status: metav1.ConditionTrue},
			},
		},
	}
}

func sampleOptions(objects ...runtime.Object) (Options, *kubefake.Clientset, *cpfake.Clientset) {
	k := kubefake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testInventory}})
	d := k.Discovery().(*discoveryfake.FakeDiscovery)
	d.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: cp.GroupVersion.String(),
			APIResources: []metav1.APIResource{{Name: testProfileResource, Kind: cp.ClusterProfileKind, Namespaced: true}},
		},
	}
	d.FakedServerVersion = &version.Info{GitVersion: testVersion}
	c := cpfake.NewSimpleClientset(objects...)
	return Options{
		Namespace:         testInventory,
		ClusterManager:    testManager,
		Kubernetes:        k,
		ClusterInventory:  c,
		SupportedFeatures: SupportedFeatureNames(),
		RunMode:           Submission,
		Suite:             Source{Version: "v0.1.0", Revision: strings.Repeat("b", 40)},
		Implementation: Implementation{
			Organization: "example",
			Project:      testManager,
			Source:       Source{Version: "v1.0.0", Revision: strings.Repeat("a", 40)},
			Mode:         "default",
			URL:          "https://example.com/manager",
			Contact:      []string{"@maintainer"},
			Reproduction: "https://example.com/manager/commit/" + strings.Repeat("a", 40),
		},
	}, k, c
}

func findResult(t *testing.T, r ConformanceReport, id string) TestResult {
	t.Helper()
	for _, test := range r.Profiles[0].Tests {
		if test.ID == id {
			return test
		}
	}
	t.Fatalf("missing check %s", id)
	return TestResult{}
}

func TestUnitRunObservesWithoutWriting(t *testing.T) {
	o, k, c := sampleOptions(sampleProfile("a", testInventory, "id-a"), sampleProfile("b", testInventory, "id-b"))
	o.ExpectedClusters = 2
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != Success || r.Profiles[0].Statistics.Passed != len(Catalog()) {
		t.Fatalf("unexpected report: %+v", r)
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
	if len(o.ReportFormats) != 0 || o.Implementation.Mode != "default" {
		t.Fatal("options were mutated")
	}
}

func TestUnitReadOnlyFailuresAndScope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Options, *kubefake.Clientset, *cpfake.Clientset, *cp.ClusterProfile)
		id     string
		out    Outcome
		result Result
	}{
		{
			"missing API",
			func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) { k.Resources = nil },
			CheckAPIAvailability,
			Failed,
			Failure,
		},
		{"cluster scoped API", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			k.Resources[0].APIResources[0].Namespaced = false
		}, CheckNamespaced, Failed, Failure},
		{"forbidden inventory", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			c.PrependReactor("list", testProfileResource, func(clienttesting.Action) (bool, runtime.Object, error) {
				resource := schema.GroupResource{Group: cp.GroupVersion.Group, Resource: testProfileResource}
				return true, nil, apierrors.NewForbidden(resource, testInventory, errors.New("denied"))
			})
		}, CheckRepresentation, Unknown, Undetermined},
		{"wrong manager", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			o.ClusterManager = testOther
		}, CheckRepresentation, Failed, Failure},
		{"count mismatch", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			o.ExpectedClusters = 2
		}, CheckRepresentation, Failed, Failure},
		{"unknown API selection", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			o.APIs = []string{"typo"}
		}, CheckAPIAvailability, Unknown, Undetermined},
		{"no tests", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			o.RunMode = Development
			o.TestFilter = "NoSuchLabel"
		}, CheckAPIAvailability, Skipped, Undetermined},
		{"skip required", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			o.RunMode = Development
			o.TestFilter = OptionalLabel
		}, CheckAPIAvailability, Skipped, Partial},
		{"submission filter", func(o *Options, k *kubefake.Clientset, c *cpfake.Clientset, p *cp.ClusterProfile) {
			o.TestFilter = RequiredLabel
		}, CheckAPIAvailability, Unknown, Undetermined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleProfile("a", testInventory, "id-a")
			o, k, c := sampleOptions(p)
			tc.change(&o, k, c, p)
			r, err := Run(context.Background(), o)
			if err == nil || r.Result != tc.result || findResult(t, r, tc.id).Outcome != tc.out {
				t.Fatalf("result=%s err=%v check=%+v", r.Result, err, findResult(t, r, tc.id))
			}
		})
	}
	o, _, _ := sampleOptions()
	r, err := Run(context.Background(), o)
	if err == nil || r.Result != Failure {
		t.Fatalf("empty inventory passed: %+v %v", r, err)
	}
}

func TestUnitNamespaceRBAC(t *testing.T) {
	o, k, _ := sampleOptions(sampleProfile("a", testInventory, "id-a"))
	k.PrependReactor("get", "namespaces", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "namespaces"},
			testInventory,
			errors.New("denied"),
		)
	})
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
}

func TestUnitConditionalAndClaimedRequirements(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*cp.ClusterProfile)
		features []string
		result   Result
		id       string
		out      Outcome
	}{
		{
			"optional label absent",
			func(p *cp.ClusterProfile) { delete(p.Labels, cp.LabelClusterManagerKey) },
			nil,
			Success,
			CheckManagerPresence,
			NotSupported,
		},
		{
			"conditional label invalid",
			func(p *cp.ClusterProfile) { p.Labels[cp.LabelClusterManagerKey] = "wrong" },
			nil,
			Failure,
			CheckManagerValue,
			Failed,
		},
		{
			"empty ID",
			func(p *cp.ClusterProfile) { p.Labels[cp.LabelInventoryMemberIDKey] = "" },
			nil,
			Failure,
			CheckMemberIDValue,
			Failed,
		},
		{
			"unclaimed recommendation",
			func(p *cp.ClusterProfile) { delete(p.Labels, cp.LabelInventoryMemberIDKey) },
			nil,
			Success,
			CheckMemberIDPresence,
			Failed,
		},
		{
			"claimed missing ID",
			func(p *cp.ClusterProfile) { delete(p.Labels, cp.LabelInventoryMemberIDKey) },
			[]string{InventoryMemberID},
			Failure,
			CheckMemberIDPresence,
			Failed,
		},
		{
			"claimed missing version",
			func(p *cp.ClusterProfile) { p.Status.Version.Kubernetes = "" },
			[]string{KubernetesVersion},
			Failure,
			CheckVersionPresence,
			NotSupported,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleProfile("a", testInventory, "id-a")
			tc.mutate(p)
			o, _, _ := sampleOptions(p)
			o.SupportedFeatures = tc.features
			r, err := Run(context.Background(), o)
			if r.Result != tc.result || findResult(t, r, tc.id).Outcome != tc.out ||
				(err == nil) != (tc.result == Success) {
				t.Fatalf("result=%s err=%v check=%+v", r.Result, err, findResult(t, r, tc.id))
			}
		})
	}
}

func TestUnitInventoryWideUniqueness(t *testing.T) {
	for _, tc := range []struct {
		name     string
		profiles []*cp.ClusterProfile
		want     Outcome
	}{
		{
			"duplicates across managers",
			[]*cp.ClusterProfile{sampleProfile("a", testInventory, "same"), sampleProfile("b", testInventory, "same")},
			Failed,
		},
		{
			"duplicate despite missing ID",
			[]*cp.ClusterProfile{
				sampleProfile("a", testInventory, "same"),
				sampleProfile("b", testInventory, "same"),
				sampleProfile("c", testInventory, ""),
			},
			Failed,
		},
		{
			"missing ID",
			[]*cp.ClusterProfile{sampleProfile("a", testInventory, "id-a"), sampleProfile("b", testInventory, "")},
			Skipped,
		},
		{
			"other inventory",
			[]*cp.ClusterProfile{sampleProfile("a", testInventory, "same"), sampleProfile("b", testOther, "same")},
			Passed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := []runtime.Object{}
			for i, p := range tc.profiles {
				if i == 1 && tc.name == "duplicates across managers" {
					p.Spec.ClusterManager.Name = testOther
					p.Labels[cp.LabelClusterManagerKey] = testOther
				}
				objects = append(objects, p)
			}
			o, _, _ := sampleOptions(objects...)
			o.RunMode = Development
			o.SupportedFeatures = nil
			r, _ := Run(context.Background(), o)
			if got := findResult(t, r, InventoryMemberUniqueness); got.Outcome != tc.want {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestUnitCancelledObservation(t *testing.T) {
	o, _, c := sampleOptions(sampleProfile("a", testInventory, "id-a"))
	c.PrependReactor(
		"list",
		testProfileResource,
		func(clienttesting.Action) (bool, runtime.Object, error) { return true, nil, context.Canceled },
	)
	r, err := Run(context.Background(), o)
	if err == nil || r.Result != Undetermined {
		t.Fatalf("%s %v", r.Result, err)
	}
}

type observationResult struct {
	report ConformanceReport
	err    error
}

func startObservation(ctx context.Context, o Options) <-chan observationResult {
	done := make(chan observationResult, 1)
	go func() {
		r, err := Run(ctx, o)
		done <- observationResult{report: r, err: err}
	}()
	return done
}

func waitObservation(t *testing.T, done <-chan observationResult) observationResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("observation did not stop")
		return observationResult{}
	}
}

func observationServer(t *testing.T, intercept func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if intercept(w, r) {
			return
		}
		var body any
		switch r.URL.Path {
		case testVersionPath:
			body = &version.Info{GitVersion: testVersion}
		case "/api/v1/namespaces/" + testInventory:
			body = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testInventory}}
		case "/apis/" + cp.GroupVersion.String():
			body = &metav1.APIResourceList{
				GroupVersion: cp.GroupVersion.String(),
				APIResources: []metav1.APIResource{{Name: testProfileResource, Kind: cp.ClusterProfileKind, Namespaced: true}},
			}
		case "/apis/" + cp.GroupVersion.String() + "/namespaces/" + testInventory + "/clusterprofiles":
			body = &cp.ClusterProfileList{
				Items: []cp.ClusterProfile{*sampleProfile("a", testInventory, "id-a")},
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil && r.Context().Err() == nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestUnitRunDiscoveryCancellation(t *testing.T) {
	for _, connection := range []string{"configured", "injected"} {
		for _, path := range []string{testVersionPath, "/apis/" + cp.GroupVersion.String()} {
			for _, cause := range []string{"timeout", "cancel", "parent deadline"} {
				t.Run(connection+path+"/"+cause, func(t *testing.T) {
					checkDiscoveryCancellation(t, connection, path, cause)
				})
			}
		}
	}
}

func checkDiscoveryCancellation(t *testing.T, connection, path, cause string) {
	t.Helper()
	entered, stopped, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	server := observationServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != path {
			return false
		}
		select {
		case <-release:
			return false
		default:
		}
		close(entered)
		select {
		case <-r.Context().Done():
			close(stopped)
		case <-release:
		}
		return true
	})
	o, _, _ := sampleOptions()
	cfg := &rest.Config{Host: server.URL, Timeout: 10 * time.Second}
	httpClient := &http.Client{Timeout: cfg.Timeout}
	if connection == "configured" {
		o.Kubernetes, o.ClusterInventory, o.Config = nil, nil, cfg
	} else {
		var err error
		o.Kubernetes, err = kubernetes.NewForConfigAndClient(cfg, httpClient)
		if err != nil {
			t.Fatal(err)
		}
		o.ClusterInventory, err = cpclient.NewForConfigAndClient(cfg, httpClient)
		if err != nil {
			t.Fatal(err)
		}
	}
	o.Timeout = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	switch cause {
	case "timeout":
		o.Timeout = 200 * time.Millisecond
	case "parent deadline":
		var deadlineCancel context.CancelFunc
		ctx, deadlineCancel = context.WithTimeout(ctx, 200*time.Millisecond)
		defer deadlineCancel()
	}
	done := startObservation(ctx, o)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery request did not start")
	}
	if cause == "cancel" {
		cancel()
	}
	result := waitObservation(t, done)
	if result.err == nil || result.report.Result != Undetermined {
		t.Fatalf("result=%s err=%v", result.report.Result, result.err)
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery HTTP request was not canceled")
	}
	if cfg.Timeout != 10*time.Second || httpClient.Timeout != cfg.Timeout {
		t.Fatal("caller-owned connection settings were changed")
	}
	unblock()
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatalf("connection could not be reused after cancellation: %v", err)
	}
}

func TestUnitRunDiscoverySharesDeadline(t *testing.T) {
	entered, stopped, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(release)
	server := observationServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case testVersionPath:
			select {
			case <-time.After(600 * time.Millisecond):
				return false
			case <-r.Context().Done():
				return true
			}
		case "/apis/" + cp.GroupVersion.String():
			close(entered)
			select {
			case <-r.Context().Done():
				close(stopped)
			case <-release:
			}
			return true
		}
		return false
	})
	o, _, _ := sampleOptions()
	o.Kubernetes, o.ClusterInventory = nil, nil
	o.Config = &rest.Config{Host: server.URL}
	o.Timeout = time.Second
	done := startObservation(context.Background(), o)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("resource discovery did not start")
	}
	select {
	case <-stopped:
	case <-time.After(750 * time.Millisecond):
		t.Fatal("resource discovery received a fresh timeout instead of the remaining run deadline")
	}
	result := waitObservation(t, done)
	if result.err == nil || result.report.Result != Undetermined {
		t.Fatalf("result=%s err=%v", result.report.Result, result.err)
	}
}

func TestUnitRunBlockedFakeDiscovery(t *testing.T) {
	for _, resource := range []string{"version", "resource"} {
		t.Run(resource, func(t *testing.T) {
			o, k, _ := sampleOptions()
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			k.PrependReactor("get", resource, func(clienttesting.Action) (bool, runtime.Object, error) {
				close(entered)
				<-release
				defer close(finished)
				return true, nil, context.Canceled
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := startObservation(ctx, o)
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("fake discovery did not start")
			}
			cancel()
			result := waitObservation(t, done)
			if result.err == nil || result.report.Result != Undetermined {
				t.Fatalf("result=%s err=%v", result.report.Result, result.err)
			}
			// Release the context-free call after Run returns, without changing its report.
			before, err := json.Marshal(result.report)
			if err != nil {
				t.Fatal(err)
			}
			unblock()
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("fake discovery did not finish after release")
			}
			after, err := json.Marshal(result.report)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("late discovery result changed the returned report")
			}
		})
	}
}

func TestUnitFlagHelp(t *testing.T) {
	fs := flag.NewFlagSet("observer", flag.ContinueOnError)
	var output bytes.Buffer
	fs.SetOutput(&output)
	RegisterFlags(fs, &Options{})
	if err := fs.Parse([]string{"-help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help error: %v", err)
	}
	if output.Len() == 0 || strings.Contains(output.String(), "panic calling String") {
		t.Fatalf("invalid help output: %s", output.String())
	}
}

func TestUnitLegacyFilters(t *testing.T) {
	const legacyRequired = "--ginkgo.label-filter=Required"
	for _, owner := range []string{"observer", "ginkgo"} {
		for _, tc := range []struct {
			name      string
			args      []string
			mode      string
			selection string
			result    Result
			wantErr   string
		}{
			{name: "unfiltered"},
			{name: "legacy", args: []string{legacyRequired}, selection: RequiredLabel},
			{name: "current", args: []string{"--test-filter=Required"}, selection: RequiredLabel},
			{
				name:      "combined",
				args:      []string{legacyRequired, "--test-filter=Optional || " + CheckManagerValue},
				selection: CheckManagerValue, result: Partial,
			},
			{
				name:      "reverse order",
				args:      []string{"--test-filter=Optional || " + CheckManagerValue, legacyRequired},
				selection: CheckManagerValue, result: Partial,
			},
			{name: "no match", args: []string{"--ginkgo.label-filter=NoSuchLabel"}, wantErr: "no checks selected"},
			{
				name:    "disjoint",
				args:    []string{legacyRequired, "--test-filter=Optional"},
				wantErr: "no checks selected",
			},
			{
				name: "submission", args: []string{legacyRequired}, mode: Submission,
				wantErr: "submission runs cannot filter checks",
			},
		} {
			t.Run(owner+"/"+tc.name, func(t *testing.T) {
				fs := flag.NewFlagSet("observer", flag.ContinueOnError)
				if owner == "ginkgo" {
					fs.String("ginkgo.label-filter", "", "Ginkgo-owned filter")
					fs.Bool("ginkgo.v", false, "Ginkgo-owned verbosity")
				}
				o, _, _ := sampleOptions(sampleProfile("a", testInventory, "id-a"))
				o.RunMode = Development
				o.SupportedFeatures = nil
				if tc.mode != "" {
					o.RunMode = tc.mode
				}
				RegisterFlags(fs, &o)
				registerLegacyFlags(fs)
				if err := fs.Parse(tc.args); err != nil {
					t.Fatal(err)
				}
				originalFilter := o.TestFilter
				r, err := Run(context.Background(), optionsWithLegacyFilter(fs, o))
				if o.TestFilter != originalFilter {
					t.Fatal("filter changed in caller-owned options")
				}
				if tc.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("expected %q, got %v", tc.wantErr, err)
					}
					return
				}
				wantResult := tc.result
				if wantResult == "" {
					wantResult = Success
				}
				if r.Result != wantResult || (err == nil) != (wantResult == Success) {
					t.Fatalf("result=%s err=%v", r.Result, err)
				}
				for _, check := range r.Profiles[0].Tests {
					want := Skipped
					if tc.selection == "" || check.ID == tc.selection ||
						(tc.selection == RequiredLabel && check.Feature == "") {
						want = Passed
					}
					if check.Outcome != want {
						t.Fatalf("unexpected selection: %+v", check)
					}
				}
			})
		}
	}
}

func TestUnitReportRoundTripAndAtomicFailure(t *testing.T) {
	o, _, _ := sampleOptions(sampleProfile("a", testInventory, "id-a"))
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = WriteReports(dir, []string{ReportFormatHTML, ReportFormatYAML}, true, r); err != nil {
		t.Fatal(err)
	}
	parsed, err := ReadReport(filepath.Join(dir, "report-clusterprofile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateReport(parsed, true); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(dir, "report-clusterprofile.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range parsed.Profiles[0].Tests {
		if !strings.Contains(string(html), test.ID) || !strings.Contains(string(html), string(test.Outcome)) {
			t.Fatalf("HTML omits check outcome: %s", test.ID)
		}
	}
	if err = WriteReports(dir, []string{"bad"}, false, r); err == nil {
		t.Fatal("invalid format accepted")
	}
	if err = os.Mkdir(filepath.Join(dir, "report.yaml"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = WriteReports(dir, []string{ReportFormatYAML}, false, r); err == nil {
		t.Fatal("output error ignored")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".conformance-") {
			t.Fatal("temporary output leaked")
		}
	}
}
