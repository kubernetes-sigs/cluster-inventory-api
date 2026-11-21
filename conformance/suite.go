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

const excludedByFilter = "Excluded by test filter"

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

func validateInventory(namespace string, expectedClusters int) error {
	if namespace == "" {
		return errors.New("namespace is required: select an inventory managed by the implementation")
	}
	if expectedClusters < 0 {
		return errors.New("expected-clusters must not be negative")
	}
	return nil
}

func validateRunMode(mode string) error {
	if mode != Development && mode != Submission {
		return fmt.Errorf("unknown run-mode %q", mode)
	}
	return nil
}

// selectProfiles resolves --apis and --profiles. With neither set only the
// default profile runs, so existing invocations keep their meaning.
func selectProfiles(o Options) ([]profile, error) {
	for _, api := range o.APIs {
		if !slices.ContainsFunc(registry, func(p profile) bool { return p.api == api }) {
			return nil, fmt.Errorf("unknown API %q", api)
		}
	}
	for _, name := range o.Profiles {
		if _, ok := lookupProfile(name); !ok {
			return nil, fmt.Errorf("unknown profile %q", name)
		}
	}
	if len(o.APIs) == 0 && len(o.Profiles) == 0 {
		return registry[:1], nil
	}
	selected := slices.DeleteFunc(slices.Clone(registry), func(p profile) bool {
		return len(o.APIs) > 0 && !slices.Contains(o.APIs, p.api) ||
			len(o.Profiles) > 0 && !slices.Contains(o.Profiles, p.name)
	})
	if len(selected) == 0 {
		return nil, errors.New("the selected APIs and profiles do not match any profile")
	}
	return selected, nil
}

func validateOptions(o Options, selected []profile) error {
	allowed := []string{}
	for _, p := range selected {
		if err := p.validateEnvironment(p.environment(o)); err != nil {
			return err
		}
		allowed = append(allowed, p.features...)
	}
	if o.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if err := validateRunMode(o.RunMode); err != nil {
		return err
	}
	if err := validateFeatures(o.SupportedFeatures, allowed); err != nil {
		return err
	}
	if err := validateFormats(o.ReportFormats); err != nil {
		return err
	}
	if len(selected) > 1 && !o.ReportPerAPI {
		return errors.New("each profile has its own report; select one profile or use report-output per-api")
	}
	if o.RunMode == Submission && o.TestFilter != "" {
		return errors.New("submission runs cannot filter checks; select APIs, profiles, and supported features instead")
	}
	return nil
}

func validateFeatures(names, allowed []string) error {
	seen := map[string]bool{}
	for _, f := range names {
		if !slices.Contains(allowed, f) {
			return fmt.Errorf("supported feature %q is not offered by the selected profiles", f)
		}
		if seen[f] {
			return fmt.Errorf("duplicate supported feature %q", f)
		}
		seen[f] = true
	}
	if seen[InventoryMemberUniqueness] && !seen[InventoryMemberID] {
		return errors.New("InventoryMemberUniqueness requires InventoryMemberID support")
	}
	return nil
}

func newReport(o Options, p profile) ConformanceReport {
	supported := slices.DeleteFunc(slices.Clone(o.SupportedFeatures), func(f string) bool {
		return !slices.Contains(p.features, f)
	})
	slices.Sort(supported)
	return ConformanceReport{
		APIVersion: ReportAPIVersion, Kind: ReportKind, Date: time.Now().UTC().Format(time.RFC3339Nano), RunMode: o.RunMode,
		Implementation: o.Implementation,
		Suite:          SuiteInfo{Source: o.Suite, CatalogDigest: p.digest()},
		Specification:  p.spec,
		API:            APIInfo{GroupVersion: cp.GroupVersion.String(), Revision: APIRevision},
		Environment:    p.environment(o),
		Profiles: []ProfileReport{
			{
				Name:              p.name,
				API:               p.api,
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

// Run observes one profile and returns its report; use RunAll to select more
// than one. It never registers clusters, writes resources, modifies Options,
// or retries until an implementation appears conformant.
func Run(ctx context.Context, options Options) (ConformanceReport, error) {
	if selected, err := selectProfiles(options); err == nil && len(selected) > 1 {
		return ConformanceReport{}, errors.New("more than one profile is selected; use RunAll")
	}
	reports, err := RunAll(ctx, options)
	return reports[0], err
}

// RunAll observes each selected profile and returns one report per profile.
func RunAll(ctx context.Context, options Options) ([]ConformanceReport, error) {
	o := defaults(options)
	selected, invalid := selectProfiles(o)
	if invalid != nil {
		selected = registry[:1]
	} else {
		invalid = validateOptions(o, selected)
	}
	reports := make([]ConformanceReport, 0, len(selected))
	errs := make([]error, 0, len(selected))
	for _, p := range selected {
		report, err := run(ctx, o, p, invalid)
		reports = append(reports, report)
		errs = append(errs, err)
	}
	if invalid != nil {
		return reports, invalid
	}
	return reports, errors.Join(errs...)
}

// run observes one profile; invalid is a setup error already found in the options.
func run(ctx context.Context, o Options, p profile, invalid error) (ConformanceReport, error) {
	report := newReport(o, p)
	abort := func(err error) (ConformanceReport, error) {
		report.Errors = append(report.Errors, err.Error())
		finish(&report)
		return report, err
	}
	fillAll := func(state Outcome, message string) {
		for _, req := range p.catalog {
			report.Profiles[0].Tests = append(report.Profiles[0].Tests, outcome(req, state, message))
		}
	}
	failSetup := func(err error) (ConformanceReport, error) {
		fillAll(Unknown, err.Error())
		return abort(err)
	}
	if invalid != nil {
		return failSetup(invalid)
	}
	filter, err := ginkgotypes.ParseLabelFilter(o.TestFilter)
	if err != nil {
		return failSetup(fmt.Errorf("invalid test filter: %w", err))
	}
	selected := func(req Requirement) bool {
		level := RequiredLabel
		if req.Optional() {
			level = OptionalLabel
		}
		return filter([]string{req.ID, level, p.api, p.name, req.Feature})
	}
	if !slices.ContainsFunc(p.catalog, selected) {
		fillAll(Skipped, excludedByFilter)
		return abort(errors.New("no checks selected"))
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
	if info, verr := serverVersion(ctx, d); verr == nil && info != nil {
		report.Environment.KubernetesVersion = info.GitVersion
	}
	if err = ctx.Err(); err != nil {
		return failSetup(err)
	}
	check, err := p.observe(ctx, o, &report)
	if err != nil {
		return failSetup(err)
	}
	for _, req := range p.catalog {
		result := outcome(req, Skipped, excludedByFilter)
		if selected(req) {
			result = check(req)
		}
		report.Profiles[0].Tests = append(report.Profiles[0].Tests, result)
	}
	finish(&report)
	if o.RunMode == Submission {
		if err := ValidateReport(report, true); err != nil {
			return abort(err)
		}
	}
	if report.Result != Success {
		return report, fmt.Errorf("conformance result: %s", report.Result)
	}
	return report, nil
}

// readNamespace fails setup when a selected namespace is missing. Forbidden is
// tolerated because observing the objects in it does not need namespace access.
func readNamespace(ctx context.Context, o Options, role, namespace string) error {
	if _, err := o.Kubernetes.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{}); err != nil &&
		!apierrors.IsForbidden(err) {
		return fmt.Errorf("read %s namespace %q: %w", role, namespace, err)
	}
	return nil
}

func observeClusterProfiles(
	ctx context.Context,
	o Options,
	r *ConformanceReport,
) (func(Requirement) TestResult, error) {
	if err := readNamespace(ctx, o, "inventory", o.Namespace); err != nil {
		return nil, err
	}
	s := snapshot{expected: o.ExpectedClusters}
	s.resources, s.apiError = serverResources(ctx, o.Kubernetes.Discovery())
	if s.apiError != nil {
		s.profileError = s.apiError
	} else {
		var list *cp.ClusterProfileList
		list, s.profileError = o.ClusterInventory.ApisV1alpha2().ClusterProfiles(o.Namespace).List(ctx, metav1.ListOptions{})
		if s.profileError == nil {
			s.profiles = list.Items
		}
	}
	slices.SortFunc(s.profiles, func(a, b cp.ClusterProfile) int { return strings.Compare(a.Name, b.Name) })
	s.managed = slices.DeleteFunc(slices.Clone(s.profiles), func(p cp.ClusterProfile) bool {
		return o.ClusterManager != "" && p.Spec.ClusterManager.Name != o.ClusterManager
	})
	for _, p := range s.profiles {
		r.Environment.ObservedProfiles = append(r.Environment.ObservedProfiles, referenceTo(p.ObjectMeta))
	}
	return func(req Requirement) TestResult { return check(req, s) }, nil
}

func finish(r *ConformanceReport) {
	for i := range r.Profiles {
		p := &r.Profiles[i]
		p.Statistics, p.Result = summarize(p.Tests, p.SupportedFeatures)
	}
	r.Result = overall(r)
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
	reports, err := RunAll(ctx, o)
	for _, report := range reports {
		if writeErr := WriteReports(o.ReportDirectory, o.ReportFormats, o.ReportPerAPI, report); writeErr != nil {
			t.Errorf("write conformance report: %v", writeErr)
		}
		p := report.Profiles[0]
		t.Logf("%s: %s (%d checks)", p.Name, report.Result, p.Statistics.Total)
	}
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
	fs.StringVar(
		&o.DecisionNamespace,
		"decision-namespace",
		o.DecisionNamespace,
		"namespace a scheduler publishes PlacementDecisions to",
	)
	fs.StringVar(
		&o.SchedulerName,
		"scheduler-name",
		o.SchedulerName,
		"schedulerName under test; empty observes all schedulers",
	)
	apis, names := make([]string, 0, len(registry)), make([]string, 0, len(registry))
	for _, p := range registry {
		apis, names = append(apis, p.api), append(names, p.name)
	}
	fs.Var(csvFlag{&o.APIs}, "apis", "APIs to observe ("+strings.Join(apis, ", ")+"); default "+apis[0])
	fs.Var(csvFlag{&o.Profiles}, "profiles", "conformance profiles ("+strings.Join(names, ", ")+"); default "+names[0])
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
