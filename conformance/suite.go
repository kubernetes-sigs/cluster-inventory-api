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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	ginkgotypes "github.com/onsi/ginkgo/v2/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	cp "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
	cpclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

func defaults(o Options) Options {
	if o.RunMode == "" {
		o.RunMode = Development
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.Implementation.Mode == "" {
		o.Implementation.Mode = "default"
	}
	if o.Suite.Version == "" {
		o.Suite.Version = "development"
	}
	if len(o.ReportFormats) == 0 {
		o.ReportFormats = []string{ReportFormatHTML}
	}
	if o.ReportDirectory == "" {
		o.ReportDirectory = "."
	}
	return o
}

func validateOptions(o Options) error {
	if o.Namespace == "" {
		return errors.New("namespace is required: select an inventory managed by the implementation")
	}
	if o.ExpectedClusters < 0 {
		return errors.New("expected-clusters must not be negative")
	}
	if o.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if o.RunMode != Development && o.RunMode != Submission {
		return fmt.Errorf("unknown run-mode %q", o.RunMode)
	}
	for _, api := range o.APIs {
		if api != ClusterProfileAPI {
			return fmt.Errorf("unknown API %q", api)
		}
	}
	for _, profile := range o.Profiles {
		if profile != ProducerProfile {
			return fmt.Errorf("unknown profile %q", profile)
		}
	}
	seen := map[string]bool{}
	for _, f := range o.SupportedFeatures {
		if !slices.Contains(features, f) {
			return fmt.Errorf("unknown supported feature %q", f)
		}
		if seen[f] {
			return fmt.Errorf("duplicate supported feature %q", f)
		}
		seen[f] = true
	}
	if seen[InventoryMemberUniqueness] && !seen[InventoryMemberID] {
		return errors.New("InventoryMemberUniqueness requires InventoryMemberID support")
	}
	for _, format := range o.ReportFormats {
		if format != ReportFormatHTML && format != ReportFormatYAML {
			return fmt.Errorf("unknown report format %q", format)
		}
	}
	if o.RunMode == Submission && o.TestFilter != "" {
		return errors.New("submission runs cannot filter checks; select APIs, profiles, and supported features instead")
	}
	return nil
}

func newReport(o Options) ConformanceReport {
	supported := slices.Clone(o.SupportedFeatures)
	slices.Sort(supported)
	return ConformanceReport{
		APIVersion: ReportAPIVersion, Kind: ReportKind, Date: time.Now().UTC().Format(time.RFC3339Nano), RunMode: o.RunMode,
		Implementation: o.Implementation,
		Suite:          SuiteInfo{Source: o.Suite, CatalogDigest: CatalogDigest()},
		Specification:  Specification{Revision: SpecRevision, URL: specRef("")},
		API:            APIInfo{GroupVersion: cp.GroupVersion.String(), Revision: APIRevision},
		Environment: Environment{
			InventoryNamespace: o.Namespace,
			ClusterManager:     o.ClusterManager,
			ExpectedClusters:   o.ExpectedClusters,
			ObservedProfiles:   []ObjectReference{},
		},
		Profiles: []ProfileReport{
			{
				Name:              ProducerProfile,
				API:               ClusterProfileAPI,
				Role:              RoleProducer,
				SupportedFeatures: supported,
				Tests:             []TestResult{},
			},
		},
		Result: Undetermined,
	}
}

func connect(o Options) (Options, error) {
	if o.Kubernetes != nil && o.ClusterInventory != nil {
		return o, nil
	}
	cfg := o.Config
	var err error
	if cfg == nil {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		rules.ExplicitPath = o.Kubeconfig
		overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults, CurrentContext: o.Context}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
		if err != nil {
			return o, fmt.Errorf("load kubeconfig: %w", err)
		}
	}
	cfg = rest.CopyConfig(cfg)
	cfg.Timeout = o.Timeout
	if o.Kubernetes == nil {
		o.Kubernetes, err = kubernetes.NewForConfig(cfg)
		if err != nil {
			return o, err
		}
	}
	if o.ClusterInventory == nil {
		o.ClusterInventory, err = cpclient.NewForConfig(cfg)
		if err != nil {
			return o, err
		}
	}
	return o, nil
}

// DiscoveryInterface has no context parameters. For custom implementations,
// stop waiting on cancellation; the buffered result lets the call finish later.
func awaitDiscovery[T any](ctx context.Context, observe func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, err := observe()
		done <- result{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case r := <-done:
		return r.value, r.err
	}
}

func serverVersion(ctx context.Context, d discovery.DiscoveryInterface) (*version.Info, error) {
	if client, ok := d.(*discovery.DiscoveryClient); ok {
		body, err := client.RESTClient().Get().AbsPath("/version").Do(ctx).Raw()
		if err != nil {
			return nil, err
		}
		var info version.Info
		if err := json.Unmarshal(body, &info); err != nil {
			return nil, fmt.Errorf("decode server version: %w", err)
		}
		return &info, nil
	}
	return awaitDiscovery(ctx, d.ServerVersion)
}

func serverResources(ctx context.Context, d discovery.DiscoveryInterface) (*metav1.APIResourceList, error) {
	if client, ok := d.(*discovery.DiscoveryClient); ok {
		resources := &metav1.APIResourceList{GroupVersion: cp.GroupVersion.String()}
		err := client.RESTClient().Get().AbsPath("/apis/" + cp.GroupVersion.String()).Do(ctx).Into(resources)
		return resources, err
	}
	return awaitDiscovery(ctx, func() (*metav1.APIResourceList, error) {
		return d.ServerResourcesForGroupVersion(cp.GroupVersion.String())
	})
}

// Run observes one inventory. It never registers clusters, writes resources,
// modifies Options, or retries until an implementation appears conformant.
func Run(ctx context.Context, options Options) (ConformanceReport, error) {
	o := defaults(options)
	report := newReport(o)
	failSetup := func(err error) (ConformanceReport, error) {
		report.Errors = append(report.Errors, err.Error())
		for _, req := range catalog {
			report.Profiles[0].Tests = append(
				report.Profiles[0].Tests,
				TestResult{Requirement: req, Outcome: Unknown, Message: err.Error()},
			)
		}
		finish(&report)
		return report, err
	}
	if err := validateOptions(o); err != nil {
		return failSetup(err)
	}
	filter, err := ginkgotypes.ParseLabelFilter(o.TestFilter)
	if err != nil {
		return failSetup(fmt.Errorf("invalid test filter: %w", err))
	}
	selected := func(req Requirement) bool {
		level := OptionalLabel
		if req.Feature == "" {
			level = RequiredLabel
		}
		return filter([]string{req.ID, level, ClusterProfileAPI, ProducerProfile, req.Feature})
	}
	selectedCount := 0
	for _, req := range catalog {
		if selected(req) {
			selectedCount++
		}
	}
	if selectedCount == 0 {
		for _, req := range catalog {
			report.Profiles[0].Tests = append(
				report.Profiles[0].Tests,
				TestResult{Requirement: req, Outcome: Skipped, Message: "Excluded by test filter"},
			)
		}
		report.Errors = append(report.Errors, "no checks selected")
		finish(&report)
		return report, errors.New("no checks selected")
	}
	if o.RunMode == Submission {
		if err := validateMetadata(report); err != nil {
			return failSetup(err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	o, err = connect(o)
	if err != nil {
		return failSetup(err)
	}
	d := o.Kubernetes.Discovery()
	if version, e := serverVersion(ctx, d); e == nil && version != nil {
		report.Environment.KubernetesVersion = version.GitVersion
	}
	if err := ctx.Err(); err != nil {
		return failSetup(err)
	}
	if _, e := o.Kubernetes.CoreV1().Namespaces().Get(ctx, o.Namespace, metav1.GetOptions{}); e != nil &&
		!apierrors.IsForbidden(e) {
		return failSetup(fmt.Errorf("read inventory namespace %q: %w", o.Namespace, e))
	}
	s := snapshot{manager: o.ClusterManager, expected: o.ExpectedClusters}
	s.resources, s.apiError = serverResources(ctx, d)
	if s.apiError != nil {
		s.profileError = s.apiError
	} else {
		list, e := o.ClusterInventory.ApisV1alpha2().ClusterProfiles(o.Namespace).List(ctx, metav1.ListOptions{})
		s.profileError = e
		if e == nil {
			s.profiles = list.Items
		}
	}
	slices.SortFunc(s.profiles, func(a, b cp.ClusterProfile) int { return strings.Compare(a.Name, b.Name) })
	for _, p := range s.profiles {
		report.Environment.ObservedProfiles = append(
			report.Environment.ObservedProfiles,
			ObjectReference{
				Namespace:       p.Namespace,
				Name:            p.Name,
				UID:             string(p.UID),
				ResourceVersion: p.ResourceVersion,
			},
		)
	}
	for _, req := range catalog {
		result := TestResult{Requirement: req, Outcome: Skipped, Message: "Excluded by test filter"}
		if selected(req) {
			result = check(req, s)
		}
		report.Profiles[0].Tests = append(report.Profiles[0].Tests, result)
	}
	finish(&report)
	if o.RunMode == Submission {
		if err := ValidateReport(report, true); err != nil {
			report.Errors = append(report.Errors, err.Error())
			finish(&report)
			return report, err
		}
	}
	if report.Result != Success {
		return report, fmt.Errorf("conformance result: %s", report.Result)
	}
	return report, nil
}

func finish(r *ConformanceReport) {
	r.Result = Success
	if len(r.Profiles) == 0 {
		r.Result = Undetermined
	}
	for i := range r.Profiles {
		p := &r.Profiles[i]
		p.Statistics, p.Result = summarize(p.Tests, p.SupportedFeatures)
		r.Result = combine(r.Result, p.Result)
	}
	if len(r.Errors) > 0 {
		r.Result = combine(r.Result, Undetermined)
	}
}

// RunConformance integrates the observer and report writer with a downstream Go test.
func RunConformance(t *testing.T, options Options) {
	t.Helper()
	o := defaults(options)
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	report, err := Run(ctx, o)
	if writeErr := WriteReports(o.ReportDirectory, o.ReportFormats, o.ReportPerAPI, report); writeErr != nil {
		t.Errorf("write conformance report: %v", writeErr)
	}
	t.Logf("%s: %s (%d checks)", ProducerProfile, report.Result, report.Profiles[0].Statistics.Total)
	if err != nil {
		t.Error(err)
	}
}

type csvFlag struct{ target *[]string }

func (v csvFlag) String() string {
	if v.target == nil {
		return ""
	}
	return strings.Join(*v.target, ",")
}

func (v csvFlag) Set(s string) error { *v.target = splitList(s); return nil }
func splitList(s string) []string {
	out := []string{}
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// RegisterFlags adds observer options to a downstream command's own FlagSet.
func RegisterFlags(fs *flag.FlagSet, o *Options) {
	*o = defaults(*o)
	fs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "kubeconfig of the inventory cluster")
	fs.StringVar(&o.Context, "context", o.Context, "kubeconfig context")
	fs.StringVar(&o.Namespace, "namespace", o.Namespace, "inventory namespace")
	fs.StringVar(
		&o.ClusterManager,
		"cluster-manager",
		o.ClusterManager,
		"spec.clusterManager.name under test; empty observes all managers",
	)
	fs.IntVar(
		&o.ExpectedClusters,
		"expected-clusters",
		o.ExpectedClusters,
		"expected number of managed profiles; zero requires at least one",
	)
	fs.Var(csvFlag{&o.APIs}, "apis", "APIs to observe (ClusterProfile)")
	fs.Var(csvFlag{&o.Profiles}, "profiles", "conformance profiles (CLUSTERPROFILE-PRODUCER)")
	fs.Var(csvFlag{&o.SupportedFeatures}, "supported-features", "optional capabilities claimed by this implementation")
	fs.StringVar(&o.TestFilter, "test-filter", o.TestFilter, "development-only label expression")
	fs.StringVar(&o.RunMode, "run-mode", o.RunMode, "development or submission")
	fs.StringVar(
		&o.Implementation.Organization,
		"organization",
		o.Implementation.Organization,
		"implementation organization",
	)
	fs.StringVar(&o.Implementation.Project, "project", o.Implementation.Project, "implementation project")
	fs.StringVar(&o.Implementation.Version, "version", o.Implementation.Version, "implementation version")
	fs.StringVar(&o.Implementation.Revision, "revision", o.Implementation.Revision, "implementation source commit")
	fs.BoolVar(
		&o.Implementation.Dirty,
		"dirty",
		o.Implementation.Dirty,
		"implementation source contains uncommitted changes",
	)
	fs.StringVar(&o.Implementation.Mode, "mode", o.Implementation.Mode, "implementation operating mode")
	fs.StringVar(&o.Implementation.URL, "url", o.Implementation.URL, "implementation URL")
	fs.Var(csvFlag{&o.Implementation.Contact}, "contact", "maintainer contacts")
	fs.StringVar(
		&o.Implementation.Reproduction,
		"reproduction",
		o.Implementation.Reproduction,
		"pinned test source or reproduction artifact URL",
	)
	fs.StringVar(&o.Suite.Version, "suite-version", o.Suite.Version, "suite module version or source snapshot")
	fs.StringVar(
		&o.Suite.Revision,
		"suite-revision",
		o.Suite.Revision,
		"suite source commit; explicit for local replacements",
	)
	fs.BoolVar(&o.Suite.Dirty, "suite-dirty", o.Suite.Dirty, "suite source contains uncommitted changes")
	fs.StringVar(&o.ReportDirectory, "report-dir", o.ReportDirectory, "report output directory")
	fs.Var(csvFlag{&o.ReportFormats}, "report-format", "html,yaml")
	fs.Func("report-output", "single or per-api", func(v string) error {
		switch v {
		case "single":
			o.ReportPerAPI = false
		case "per-api":
			o.ReportPerAPI = true
		default:
			return fmt.Errorf("unknown report-output %q", v)
		}
		return nil
	})
	fs.DurationVar(&o.Timeout, "timeout", o.Timeout, "deadline for API observations")
}

// Legacy CLI flags remain available to existing TestConformance importers.
var cliOptions Options

func init() {
	RegisterFlags(flag.CommandLine, &cliOptions)
	registerLegacyFlags(flag.CommandLine)
}

func registerLegacyFlags(fs *flag.FlagSet) {
	if fs.Lookup("ginkgo.label-filter") == nil {
		fs.String("ginkgo.label-filter", "", "alias for test-filter; combined with test-filter using AND")
	}
	if fs.Lookup("ginkgo.v") == nil {
		fs.Bool("ginkgo.v", false, "compatibility flag; Go test -v controls verbosity")
	}
}

func optionsWithLegacyFilter(fs *flag.FlagSet, o Options) Options {
	if f := fs.Lookup("ginkgo.label-filter"); f != nil && f.Value.String() != "" {
		if o.TestFilter == "" {
			o.TestFilter = f.Value.String()
		} else {
			o.TestFilter = fmt.Sprintf("(%s) && (%s)", o.TestFilter, f.Value.String())
		}
	}
	return o
}

func TestConformance(t *testing.T) {
	t.Helper()
	RunConformance(t, optionsWithLegacyFilter(flag.CommandLine, cliOptions))
}
