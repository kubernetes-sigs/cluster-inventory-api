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
	"flag"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	cpv1alpha2 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
	cpclientset "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

// api identifies one of the Cluster Inventory APIs the suite covers. Its Name
// is the Ginkgo label carried by the Describe containers of the API's specs,
// the value accepted by --apis, and the heading of the API's report section.
type api struct {
	Name string
}

var (
	clusterProfileAPI = api{Name: cpv1alpha2.ClusterProfileKind}

	// apis lists the covered APIs in the order they are reported.
	apis = []api{clusterProfileAPI}
)

const (
	reportFormatHTML = "html"
	reportFormatYAML = "yaml"

	reportOutputSingle = "single"
	reportOutputPerAPI = "per-api"
)

var (
	loadingRules   *clientcmd.ClientConfigLoadingRules
	kubeContext    string
	namespace      string
	clusterManager string
	// expectedClusters is the number of clusters registered with the cluster
	// manager under test; 0 means unknown.
	expectedClusters int

	organization string
	project      string
	version      string
	url          string

	reportFormatFlag string
	reportOutputFlag string
	apisFlag         string

	// Report options parsed from the flags above by parseReportOptions.
	reportFormats []string
	reportPerAPI  bool
	selectedAPIs  []api

	restConfig           *rest.Config
	kubernetesClient     kubernetes.Interface
	clusterProfileClient cpclientset.Interface
)

func init() {
	loadingRules = clientcmd.NewDefaultClientConfigLoadingRules()
	flag.StringVar(&loadingRules.ExplicitPath, "kubeconfig", "",
		"absolute path to the kubeconfig file of the cluster serving the Cluster Inventory APIs")
	flag.StringVar(&kubeContext, "context", "",
		"kubeconfig context of the cluster serving the Cluster Inventory APIs")
	flag.StringVar(&namespace, "namespace", "",
		"inventory namespace containing the ClusterProfile objects managed by the implementation under test; "+
			"the suite observes these objects and never creates ClusterProfiles itself")
	flag.StringVar(&clusterManager, "cluster-manager", "",
		"name of the cluster manager under test, as reported in spec.clusterManager.name; if unset, all "+
			"ClusterProfile objects in the inventory namespace are verified")
	flag.IntVar(&expectedClusters, "expected-clusters", 0,
		"number of member clusters registered with the cluster manager under test; when positive, the suite "+
			"verifies that exactly that many ClusterProfile objects exist in the inventory namespace, while 0 "+
			"(the default) leaves the count unknown and only requires at least one")
	flag.StringVar(&apisFlag, "apis", "",
		fmt.Sprintf("comma-separated list of the APIs to test (%s); if unset, all APIs are tested",
			strings.Join(apiNames(apis), ", ")))
	flag.StringVar(&reportFormatFlag, "report-format", reportFormatHTML,
		fmt.Sprintf("comma-separated list of the report formats to write (%s, %s)", reportFormatHTML, reportFormatYAML))
	flag.StringVar(&reportOutputFlag, "report-output", reportOutputSingle,
		fmt.Sprintf("report output: %s (one report covering all tested APIs) or %s (one report per API)",
			reportOutputSingle, reportOutputPerAPI))
	flag.StringVar(&organization, "organization", "",
		"name of the organization responsible for the implementation being tested")
	flag.StringVar(&project, "project", "", "name of the implementation project being tested")
	flag.StringVar(&version, "version", "", "version of the implementation being tested")
	flag.StringVar(&url, "url", "", "URL pointing to the implementation project or its documentation")
}

// TestConformance runs the Cluster Inventory API conformance suite. It is
// exported so implementations can import this package and run the suite from
// their own test pipelines.
func TestConformance(t *testing.T) {
	// An invalid API selection must fail here: Ginkgo skips the suite set up
	// when no spec would run, so a filter matching nothing would otherwise
	// pass silently.
	if err := parseReportOptions(); err != nil {
		t.Fatal(err)
	}

	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	if filter := apiLabelFilter(); filter != "" {
		if suiteConfig.LabelFilter != "" {
			suiteConfig.LabelFilter = fmt.Sprintf("(%s) && (%s)", suiteConfig.LabelFilter, filter)
		} else {
			suiteConfig.LabelFilter = filter
		}
	}

	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Cluster Inventory API Conformance Suite", suiteConfig, reporterConfig)
}

// parseReportOptions validates --report-format, --report-output and --apis and
// stores the parsed values in reportFormats, reportPerAPI and selectedAPIs.
func parseReportOptions() error {
	reportFormats = nil
	for _, format := range splitList(reportFormatFlag) {
		switch format {
		case reportFormatHTML, reportFormatYAML:
			if !slices.Contains(reportFormats, format) {
				reportFormats = append(reportFormats, format)
			}
		default:
			return fmt.Errorf("unsupported report format %q in --report-format; supported formats: %s, %s",
				format, reportFormatHTML, reportFormatYAML)
		}
	}
	if len(reportFormats) == 0 {
		return fmt.Errorf("--report-format must list at least one format: %s, %s", reportFormatHTML, reportFormatYAML)
	}

	switch reportOutputFlag {
	case reportOutputSingle:
		reportPerAPI = false
	case reportOutputPerAPI:
		reportPerAPI = true
	default:
		return fmt.Errorf("unsupported report output %q in --report-output; supported outputs: %s, %s",
			reportOutputFlag, reportOutputSingle, reportOutputPerAPI)
	}

	names := splitList(apisFlag)
	if len(names) == 0 {
		selectedAPIs = apis
		return nil
	}

	selectedAPIs = nil
	for _, name := range names {
		idx := slices.IndexFunc(apis, func(a api) bool { return strings.EqualFold(a.Name, name) })
		if idx == -1 {
			return fmt.Errorf("unknown API %q in --apis; known APIs: %s", name, strings.Join(apiNames(apis), ", "))
		}
		if !slices.Contains(selectedAPIs, apis[idx]) {
			selectedAPIs = append(selectedAPIs, apis[idx])
		}
	}

	// Report the selected APIs in registry order regardless of the flag order.
	slices.SortFunc(selectedAPIs, func(a, b api) int {
		return slices.Index(apis, a) - slices.Index(apis, b)
	})

	return nil
}

// apiLabelFilter returns the Ginkgo label filter selecting the specs of the
// selected APIs, or "" when every API is selected.
func apiLabelFilter() string {
	if len(selectedAPIs) == len(apis) {
		return ""
	}

	return strings.Join(apiNames(selectedAPIs), " || ")
}

func apiNames(list []api) []string {
	names := make([]string, 0, len(list))
	for _, a := range list {
		names = append(names, a.Name)
	}

	return names
}

func splitList(s string) []string {
	var items []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}

	return items
}

var _ = ginkgo.BeforeSuite(func(ctx context.Context) {
	gomega.Expect(setupSuite(ctx)).To(gomega.Succeed(), "Test suite set up failed")
})

func setupSuite(ctx context.Context) error {
	if namespace == "" {
		return errors.New("the --namespace flag is required: it names the inventory namespace containing " +
			"the ClusterProfile objects managed by the implementation under test")
	}

	if expectedClusters < 0 {
		return errors.New("the --expected-clusters flag must not be negative")
	}

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	overrides.CurrentContext = kubeContext

	var err error

	restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return fmt.Errorf("error building the Kubernetes client configuration: %w", err)
	}

	kubernetesClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("error creating the Kubernetes client: %w", err)
	}

	clusterProfileClient, err = cpclientset.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("error creating the ClusterProfile client: %w", err)
	}

	// The namespace is read only to fail early on a wrong --namespace value. A
	// runner holding just a namespaced Role on ClusterProfiles, the RBAC model
	// KEP-4322 supports for inventories, is not allowed to read it; the specs
	// verify access through the ClusterProfile API itself.
	_, err = kubernetesClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil && !apierrors.IsForbidden(err) {
		return fmt.Errorf("error retrieving the inventory namespace %q: %w", namespace, err)
	}

	return nil
}
