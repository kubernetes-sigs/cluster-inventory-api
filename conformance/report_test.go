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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
)

var (
	alphaAPI = api{Name: "Alpha"}
	betaAPI  = api{Name: "Beta"}
	testAPIs = []api{alphaAPI, betaAPI}
)

func fakeSpecReport(apiName, text, label string, state types.SpecState) types.SpecReport {
	return types.SpecReport{
		LeafNodeType:             types.NodeTypeIt,
		LeafNodeText:             text,
		LeafNodeLabels:           []string{label},
		ContainerHierarchyTexts:  []string{apiName},
		ContainerHierarchyLabels: [][]string{{apiName}},
		State:                    state,
		ReportEntries: types.ReportEntries{{
			Name:  SpecRefReportEntry,
			Value: types.WrapEntryValue("https://example.com/" + apiName),
		}},
	}
}

func fakeReport() ginkgo.Report {
	return ginkgo.Report{SpecReports: types.SpecReports{
		fakeSpecReport(alphaAPI.Name, "must do a", RequiredLabel, types.SpecStatePassed),
		fakeSpecReport(alphaAPI.Name, "should do a", OptionalLabel, types.SpecStateFailed),
		fakeSpecReport(betaAPI.Name, "must do b", RequiredLabel, types.SpecStatePassed),
	}}
}

func TestBuildReportGroupsSpecsByAPI(t *testing.T) {
	data, err := buildReport(fakeReport(), testAPIs, testAPIs)
	if err != nil {
		t.Fatal(err)
	}

	if got := apiReportNames(data); !slices.Equal(got, []string{"Alpha", "Beta"}) {
		t.Fatalf("expected the APIs Alpha and Beta in that order, got %v", got)
	}

	if data.Passed != 2 || data.Total != 3 {
		t.Errorf("expected 2 of 3 tests passed overall, got %d of %d", data.Passed, data.Total)
	}

	alpha := data.APIs[0]
	if alpha.Passed != 1 || alpha.Total != 2 {
		t.Errorf("expected 1 of 2 Alpha tests passed, got %d of %d", alpha.Passed, alpha.Total)
	}

	if got := groupNames(alpha); !slices.Equal(got, []string{RequiredLabel, OptionalLabel}) {
		t.Errorf("expected the Alpha groups Required and Optional in that order, got %v", got)
	}

	required := alpha.Groups[0].Tests[0]
	if required.Desc != "Alpha must do a" || !required.Passed || required.Ref != "https://example.com/Alpha" {
		t.Errorf("unexpected Required Alpha test: %+v", required)
	}

	if len(required.Labels) != 0 {
		t.Errorf("the API and reporting labels must not be listed as extra labels, got %v", required.Labels)
	}

	if optional := alpha.Groups[1].Tests[0]; !optional.Failed || optional.Passed {
		t.Errorf("expected the failed Optional Alpha test to be reported as failed, got %+v", optional)
	}

	if beta := data.APIs[1]; beta.Passed != 1 || beta.Total != 1 || len(beta.Groups) != 1 {
		t.Errorf("expected a single passed Required Beta test, got %+v", beta)
	}
}

func TestBuildReportOmitsUnselectedAPIs(t *testing.T) {
	report := fakeReport()
	// Ginkgo reports the specs filtered out by the API label filter as skipped.
	for i := range report.SpecReports {
		if report.SpecReports[i].ContainerHierarchyTexts[0] == alphaAPI.Name {
			report.SpecReports[i].State = types.SpecStateSkipped
		}
	}

	data, err := buildReport(report, testAPIs, []api{betaAPI})
	if err != nil {
		t.Fatal(err)
	}

	if got := apiReportNames(data); !slices.Equal(got, []string{"Beta"}) {
		t.Fatalf("expected only the Beta API to be reported, got %v", got)
	}

	if data.Passed != 1 || data.Total != 1 {
		t.Errorf("expected 1 of 1 tests passed, got %d of %d", data.Passed, data.Total)
	}
}

func TestBuildReportRejectsSpecWithoutAPILabel(t *testing.T) {
	unlabeled := fakeSpecReport(alphaAPI.Name, "must do a", RequiredLabel, types.SpecStatePassed)
	unlabeled.ContainerHierarchyLabels = nil

	_, err := buildReport(ginkgo.Report{SpecReports: types.SpecReports{unlabeled}}, testAPIs, testAPIs)
	if err == nil || !strings.Contains(err.Error(), "Alpha, Beta") {
		t.Fatalf("expected an error naming the known APIs, got %v", err)
	}
}

func TestBuildReportRejectsSpecWithoutExactlyOneReportingLabel(t *testing.T) {
	for name, labels := range map[string][]string{
		"no reporting label":    nil,
		"both reporting labels": {RequiredLabel, OptionalLabel},
	} {
		t.Run(name, func(t *testing.T) {
			spec := fakeSpecReport(alphaAPI.Name, "must do a", RequiredLabel, types.SpecStatePassed)
			spec.LeafNodeLabels = labels

			_, err := buildReport(ginkgo.Report{SpecReports: types.SpecReports{spec}}, testAPIs, testAPIs)
			if err == nil || !strings.Contains(err.Error(), "exactly one of the reporting labels") {
				t.Fatalf("expected an error about the reporting labels, got %v", err)
			}
		})
	}
}

func TestWriteReportsSingle(t *testing.T) {
	dir := t.TempDir()
	data, err := buildReport(fakeReport(), testAPIs, testAPIs)
	if err != nil {
		t.Fatal(err)
	}

	// The metadata fields are independent: the block must render without a project name.
	data.Implementation = implementationInfo{Organization: "Example Org", URL: "https://example.com/impl"}

	if err := writeReports(dir, []string{reportFormatHTML, reportFormatYAML}, false, data); err != nil {
		t.Fatal(err)
	}

	expectFiles(t, dir, "report.html", "report.yaml")

	html := readFile(t, filepath.Join(dir, "report.html"))
	wanted := []string{"Alpha API", "Beta API", "Alpha must do a", "Beta must do b", "https://example.com/Alpha",
		"(Example Org)", "https://example.com/impl"}
	for _, want := range wanted {
		if !strings.Contains(html, want) {
			t.Errorf("expected the single HTML report to contain %q", want)
		}
	}

	yamlReport := readFile(t, filepath.Join(dir, "report.yaml"))
	for _, want := range []string{"name: Alpha", "name: Beta", "passed: 2", "total: 3"} {
		if !strings.Contains(yamlReport, want) {
			t.Errorf("expected the single YAML report to contain %q", want)
		}
	}
}

func TestWriteReportsPerAPI(t *testing.T) {
	dir := t.TempDir()
	data, err := buildReport(fakeReport(), testAPIs, testAPIs)
	if err != nil {
		t.Fatal(err)
	}

	if err := writeReports(dir, []string{reportFormatHTML, reportFormatYAML}, true, data); err != nil {
		t.Fatal(err)
	}

	expectFiles(t, dir, "report-alpha.html", "report-alpha.yaml", "report-beta.html", "report-beta.yaml")

	alpha := readFile(t, filepath.Join(dir, "report-alpha.html"))
	if !strings.Contains(alpha, "Alpha must do a") || strings.Contains(alpha, "Beta") {
		t.Errorf("expected the Alpha HTML report to contain only Alpha tests")
	}

	beta := readFile(t, filepath.Join(dir, "report-beta.yaml"))
	if !strings.Contains(beta, "name: Beta") || strings.Contains(beta, "Alpha") ||
		!strings.Contains(beta, "passed: 1") || !strings.Contains(beta, "total: 1") {
		t.Errorf("expected the Beta YAML report to contain only Beta tests with its own totals, got:\n%s", beta)
	}
}

func apiReportNames(data reportData) []string {
	names := make([]string, 0, len(data.APIs))
	for _, a := range data.APIs {
		names = append(names, a.Name)
	}

	return names
}

func groupNames(apiRep apiReport) []string {
	names := make([]string, 0, len(apiRep.Groups))
	for _, g := range apiRep.Groups {
		names = append(names, g.Name)
	}

	return names
}

func expectFiles(t *testing.T, dir string, names ...string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	slices.Sort(got)
	slices.Sort(names)

	if !slices.Equal(got, names) {
		t.Fatalf("expected the report files %v, got %v", names, got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(content)
}
