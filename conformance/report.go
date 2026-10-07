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
	_ "embed" // Needed for go:embed
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/matchers"
	"gopkg.in/yaml.v3"
)

const (
	// RequiredLabel marks specs every implementation must pass to be conformant.
	RequiredLabel = "Required"
	// OptionalLabel marks specs for optional parts of the specification.
	OptionalLabel = "Optional"

	// SpecRefReportEntry is the report entry holding a spec's KEP reference.
	SpecRefReportEntry = "spec-ref"
	// NonConformantReportEntry is the report entry holding a spec's non-conformance message.
	NonConformantReportEntry = "non-conformant"
)

var reportingLabels = []string{
	RequiredLabel,
	OptionalLabel,
}

//go:embed report_template.gohtml
var reportHTML string

var reportTemplate = template.Must(template.New("report").Parse(reportHTML))

type testInfo struct {
	Desc       string
	Ref        string
	Labels     []string
	Passed     bool
	Failed     bool
	Skipped    bool
	Conformant bool
	Message    string
}

type testGrouping struct {
	Name  string
	Tests []testInfo
}

// apiReport holds the results of one API's specs, grouped by reporting label.
type apiReport struct {
	Name   string
	Groups []testGrouping
	Passed int
	Total  int
}

type implementationInfo struct {
	Organization string
	Project      string
	Version      string
	URL          string
}

// reportData is the content of a conformance report, rendered as HTML or YAML.
type reportData struct {
	APIs           []apiReport
	SuiteFailure   string
	Passed         int
	Total          int
	Implementation implementationInfo
}

var (
	errorRegEx *regexp.Regexp
	// currentSpecNonConformanceMsg holds the last non-conformance message emitted by the current spec. Using
	// Eventually with a function that takes a Gomega, different assertions could report non-conformance on
	// successive retries, so we want to just report the last one. This also allows the last message to be cleared
	// (via cancelNonConformanceReport()) if it eventually succeeded.
	currentSpecNonConformanceMsg atomic.Value

	// specRefRegistry maps test description substrings to the KEP spec reference.
	specRefRegistry = map[string]string{}
)

// SpecifyWithSpecRef exists to be able to register a spec's KEP reference
// alongside a Specify. This is important to be able to correctly attach the
// spec ref even if a test is skipped.
func SpecifyWithSpecRef(text string, specRef string, args ...interface{}) bool {
	specRefRegistry[text] = specRef
	return ginkgo.Specify(text, args...)
}

func lookupSpecRef(fullText string) string {
	for desc, ref := range specRefRegistry {
		if strings.Contains(fullText, desc) {
			return ref
		}
	}

	return ""
}

func init() {
	dummyErr := errors.New("dummy")
	errorRegEx = regexp.MustCompile(fmt.Sprintf(`\s*(.*)\s*(?:%s|%s|The function passed to)\s*.*\s*(.*)`,
		regexp.QuoteMeta(firstLine((&matchers.HaveOccurredMatcher{}).NegatedFailureMessage(dummyErr))),
		regexp.QuoteMeta(firstLine((&matchers.SucceedMatcher{}).FailureMessage(dummyErr)))))
}

var _ = ginkgo.ReportBeforeEach(func(specReport ginkgo.SpecReport) {
	cancelNonConformanceReport()

	if ref := lookupSpecRef(specReport.FullText()); ref != "" {
		ginkgo.AddReportEntry(SpecRefReportEntry, ref)
	}
})

var _ = ginkgo.ReportAfterEach(func(specReport ginkgo.SpecReport) {
	if specReport.LeafNodeType != types.NodeTypeIt || specReport.State == types.SpecStatePending ||
		specReport.State == types.SpecStateSkipped {
		return
	}

	msg, _ := currentSpecNonConformanceMsg.Swap("").(string)
	if msg != "" {
		ginkgo.AddReportEntry(NonConformantReportEntry, msg, types.ReportEntryVisibilityNever)
	}
})

var _ = ginkgo.ReportAfterSuite("Cluster Inventory API conformance report", func(report ginkgo.Report) {
	data, err := buildReport(report, apis, selectedAPIs)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	data.Implementation = implementationInfo{
		Organization: organization,
		Project:      project,
		Version:      version,
		URL:          url,
	}

	gomega.Expect(writeReports(".", reportFormats, reportPerAPI, data)).To(gomega.Succeed())
})

// buildReport groups the spec results by API (in the order of selected) and
// by reporting label. Every spec must carry the label of one of the known
// APIs and exactly one reporting label; specs of APIs that are not selected,
// which Ginkgo filtered out, are left out of the report rather than listed as
// skipped.
func buildReport(report ginkgo.Report, known, selected []api) (reportData, error) {
	data := reportData{}
	groupsByAPI := map[string]map[string]*testGrouping{}

	for _, specReport := range report.SpecReports {
		if specReport.LeafNodeType == types.NodeTypeBeforeSuite && specReport.State == types.SpecStateFailed {
			data.SuiteFailure = parseFailureMessage(specReport.FailureMessage())
			continue
		}

		if specReport.LeafNodeType != types.NodeTypeIt || specReport.State == types.SpecStatePending {
			continue
		}

		apiName, ok := apiOf(specReport, known)
		if !ok {
			return reportData{}, fmt.Errorf("spec %q does not carry an API label; every spec must be labeled "+
				"with one of: %s", strings.TrimSpace(specReport.FullText()), strings.Join(apiNames(known), ", "))
		}

		if !slices.ContainsFunc(selected, func(a api) bool { return a.Name == apiName }) {
			continue
		}

		label, err := reportingLabelOf(specReport)
		if err != nil {
			return reportData{}, err
		}

		if groupsByAPI[apiName] == nil {
			groupsByAPI[apiName] = map[string]*testGrouping{}
		}

		if groupsByAPI[apiName][label] == nil {
			groupsByAPI[apiName][label] = &testGrouping{
				Name: label,
			}
		}

		groupsByAPI[apiName][label].Tests = append(groupsByAPI[apiName][label].Tests,
			specTestInfo(specReport, apiName, label))
	}

	for _, a := range selected {
		apiRep := apiReport{Name: a.Name}

		for _, l := range reportingLabels {
			group := groupsByAPI[a.Name][l]
			if group == nil {
				continue
			}

			slices.SortFunc(group.Tests, func(a, b testInfo) int {
				if cmp := slices.Compare(a.Labels, b.Labels); cmp != 0 {
					return cmp
				}
				return strings.Compare(strings.TrimSpace(a.Desc), strings.TrimSpace(b.Desc))
			})
			apiRep.Groups = append(apiRep.Groups, *group)

			for _, t := range group.Tests {
				apiRep.Total++
				if t.Passed {
					apiRep.Passed++
				}
			}
		}

		data.APIs = append(data.APIs, apiRep)
		data.Passed += apiRep.Passed
		data.Total += apiRep.Total
	}

	return data, nil
}

// specTestInfo summarizes a spec's result for the report, leaving out the API
// and reporting labels it is filed under.
func specTestInfo(specReport types.SpecReport, apiName, label string) testInfo {
	info := testInfo{
		Desc:       strings.TrimSpace(specReport.FullText()),
		Conformant: true,
	}

	for _, currLabel := range specReport.Labels() {
		if currLabel != label && currLabel != apiName {
			info.Labels = append(info.Labels, currLabel)
		}
	}
	slices.Sort(info.Labels)

	for i := range specReport.ReportEntries {
		switch specReport.ReportEntries[i].Name {
		case SpecRefReportEntry:
			info.Ref = specReport.ReportEntries[i].GetRawValue().(string)
		case NonConformantReportEntry:
			// An assertion reporting non-conformance may have failed initially but eventually succeeded
			// after retries so only report non-conformance if the spec actually failed.
			if specReport.State != types.SpecStatePassed {
				info.Conformant = false
				info.Message = specReport.ReportEntries[i].GetRawValue().(string)
			}
		}
	}

	if specReport.State == types.SpecStateSkipped {
		info.Skipped = true
		info.Message = parseFailureMessage(specReport.FailureMessage())
	} else if specReport.State != types.SpecStatePassed && info.Conformant {
		// If the spec failed (ie didn't pass) not due to non-conformance then we assume it encountered
		// an unexpected error preventing conformance from being determined, and thus we'll report the
		// conformance status as unknown.
		info.Failed = true
		info.Message = parseFailureMessage(specReport.FailureMessage())
	}

	info.Passed = !info.Failed && !info.Skipped && info.Conformant

	if info.Message != "" {
		info.Message = " - " + info.Message
	}

	return info
}

// reportingLabelOf returns the reporting label of a spec, which must carry
// exactly one so that it is counted once in the report.
func reportingLabelOf(specReport types.SpecReport) (string, error) {
	found := make([]string, 0, len(reportingLabels))
	for _, label := range reportingLabels {
		if slices.Contains(specReport.Labels(), label) {
			found = append(found, label)
		}
	}

	if len(found) != 1 {
		return "", fmt.Errorf("spec %q must carry exactly one of the reporting labels %s; got %v",
			strings.TrimSpace(specReport.FullText()), strings.Join(reportingLabels, ", "), found)
	}

	return found[0], nil
}

// apiOf returns the name of the known API whose label the spec carries.
func apiOf(specReport types.SpecReport, known []api) (string, bool) {
	for _, a := range known {
		if slices.Contains(specReport.Labels(), a.Name) {
			return a.Name, true
		}
	}

	return "", false
}

// writeReports writes the report into dir in every requested format, either
// as a single report.<format> covering all APIs or, with perAPI, as one
// report-<api>.<format> per API.
func writeReports(dir string, formats []string, perAPI bool, data reportData) error {
	if !perAPI {
		return writeReport(dir, "report", formats, data)
	}

	for _, apiRep := range data.APIs {
		apiData := reportData{
			APIs:           []apiReport{apiRep},
			SuiteFailure:   data.SuiteFailure,
			Passed:         apiRep.Passed,
			Total:          apiRep.Total,
			Implementation: data.Implementation,
		}

		if err := writeReport(dir, "report-"+strings.ToLower(apiRep.Name), formats, apiData); err != nil {
			return err
		}
	}

	return nil
}

func writeReport(dir, base string, formats []string, data reportData) error {
	for _, format := range formats {
		path := filepath.Join(dir, base+"."+format)
		if err := writeReportFile(path, format, data); err != nil {
			return fmt.Errorf("error writing the %s report %s: %w", format, path, err)
		}
	}

	return nil
}

func writeReportFile(path, format string, data reportData) (err error) {
	out, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}()

	switch format {
	case reportFormatHTML:
		return reportTemplate.Execute(out, data)
	case reportFormatYAML:
		encoder := yaml.NewEncoder(out)
		if encodeErr := encoder.Encode(data); encodeErr != nil {
			return encodeErr
		}

		return encoder.Close()
	default:
		return fmt.Errorf("unsupported report format %q", format)
	}
}

func parseFailureMessage(s string) string {
	// First see if the message represents an error formatted by a gomega matcher - we're interested in
	// extracting the optional user description passed to the gomega assertion and the actual error
	// string as these are the useful parts conducive for formatting in the report table.
	matches := errorRegEx.FindStringSubmatch(s)
	if len(matches) > 0 {
		// First match at index 0 is the full text that was matched; index 1 will be the user description
		// and index 2 the error string. We concatenate the latter two.
		msg := strings.TrimSpace(matches[1])
		if msg == "" {
			msg = strings.TrimSpace(matches[2])
		} else {
			msg = strings.TrimSuffix(msg, ".") + ": " + strings.TrimSpace(matches[2])
		}

		return msg
	}

	// Fallback - just take the first line in the message.
	return firstLine(s)
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(first)
}

// reportNonConformant is intended for use as an optional description in a gomega assertion. It returns
// a function that is lazily evaluated by the assertion only if a failure occurs. We take advantage of
// that to add a report entry indicating non-conformance.
func reportNonConformant(msg string) func() string {
	return func() string {
		currentSpecNonConformanceMsg.Store(msg)
		return msg
	}
}

func cancelNonConformanceReport() {
	currentSpecNonConformanceMsg.Store("")
}
