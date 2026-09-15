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

type implementationInfo struct {
	Organization string
	Project      string
	Version      string
	URL          string
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

var _ = ginkgo.ReportAfterSuite("ClusterProfile API conformance report", func(report ginkgo.Report) {
	testGroupMap := map[string]*testGrouping{}
	suiteFailure := ""

	for _, specReport := range report.SpecReports {
		if specReport.LeafNodeType == types.NodeTypeBeforeSuite && specReport.State == types.SpecStateFailed {
			suiteFailure = parseFailureMessage(specReport.FailureMessage())
			continue
		}

		if specReport.LeafNodeType != types.NodeTypeIt || specReport.State == types.SpecStatePending {
			continue
		}

		for _, label := range reportingLabels {
			if !slices.Contains(specReport.Labels(), label) {
				continue
			}

			if testGroupMap[label] == nil {
				testGroupMap[label] = &testGrouping{
					Name: label,
				}
			}

			info := testInfo{
				Desc:       strings.TrimSpace(specReport.FullText()),
				Conformant: true,
			}

			for _, currLabel := range specReport.Labels() {
				if currLabel != label {
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

			testGroupMap[label].Tests = append(testGroupMap[label].Tests, info)
		}
	}

	testGroups := []testGrouping{}

	for _, l := range reportingLabels {
		if testGroupMap[l] != nil {
			slices.SortFunc(testGroupMap[l].Tests, func(a, b testInfo) int {
				if cmp := slices.Compare(a.Labels, b.Labels); cmp != 0 {
					return cmp
				}
				return strings.Compare(strings.TrimSpace(a.Desc), strings.TrimSpace(b.Desc))
			})
			testGroups = append(testGroups, *testGroupMap[l])
		}
	}

	totalTests := 0
	passedTests := 0

	for _, g := range testGroups {
		for _, t := range g.Tests {
			totalTests++
			if t.Passed {
				passedTests++
			}
		}
	}

	data := struct {
		Groups         []testGrouping
		SuiteFailure   string
		Passed         int
		Total          int
		Implementation implementationInfo
	}{
		Groups:       testGroups,
		SuiteFailure: suiteFailure,
		Passed:       passedTests,
		Total:        totalTests,
		Implementation: implementationInfo{
			Organization: organization,
			Project:      project,
			Version:      version,
			URL:          url,
		},
	}

	out, err := os.Create("report.html")
	gomega.Expect(err).To(gomega.Succeed())

	tmpl, err := template.New("report").Parse(reportHTML)
	gomega.Expect(err).To(gomega.Succeed())

	gomega.Expect(tmpl.Execute(out, data)).To(gomega.Succeed())
	gomega.Expect(out.Close()).To(gomega.Succeed())

	yamlOut, err := os.Create("report.yaml")
	gomega.Expect(err).To(gomega.Succeed())

	encoder := yaml.NewEncoder(yamlOut)
	gomega.Expect(encoder.Encode(data)).To(gomega.Succeed())
	gomega.Expect(encoder.Close()).To(gomega.Succeed())
	gomega.Expect(yamlOut.Close()).To(gomega.Succeed())
})

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
