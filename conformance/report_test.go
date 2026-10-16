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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func completeReport(t *testing.T) ConformanceReport {
	t.Helper()
	o, _, _ := sampleOptions(sampleProfile("a", testInventory, "id-a"))
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestUnitSubmissionAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*ConformanceReport)
	}{
		{"unknown schema", func(r *ConformanceReport) { r.APIVersion = "other/v1" }},
		{"empty metadata", func(r *ConformanceReport) { r.Implementation.Project = "" }},
		{"missing contact", func(r *ConformanceReport) { r.Implementation.Contact = nil }},
		{"dirty suite", func(r *ConformanceReport) { r.Suite.Dirty = true }},
		{"dirty implementation", func(r *ConformanceReport) { r.Implementation.Dirty = true }},
		{"invalid source revision", func(r *ConformanceReport) { r.Suite.Revision = "main" }},
		{"moving spec", func(r *ConformanceReport) { r.Specification.Revision = "master" }},
		{"wrong catalog", func(r *ConformanceReport) { r.Suite.CatalogDigest = "wrong" }},
		{"missing check", func(r *ConformanceReport) { r.Profiles[0].Tests = r.Profiles[0].Tests[1:] }},
		{"duplicate check", func(r *ConformanceReport) { r.Profiles[0].Tests[1] = r.Profiles[0].Tests[0] }},
		{"tampered count", func(r *ConformanceReport) { r.Profiles[0].Statistics.Passed++ }},
		{"tampered result", func(r *ConformanceReport) {
			r.Profiles[0].Tests[0].Outcome = Skipped
			r.Profiles[0].Tests[0].Message = "filtered"
		}},
		{"empty outcome reason", func(r *ConformanceReport) { r.Profiles[0].Tests[0].Outcome = Skipped; finish(r) }},
		{"development result", func(r *ConformanceReport) { r.RunMode = Development }},
		{"no observed profiles", func(r *ConformanceReport) { r.Environment.ObservedProfiles = nil }},
		{"unknown environment", func(r *ConformanceReport) { r.Environment.KubernetesVersion = "" }},
		{"failure", func(r *ConformanceReport) {
			r.Profiles[0].Tests[0].Outcome = Failed
			r.Profiles[0].Tests[0].Message = "not served"
			finish(r)
		}},
		{"unknown", func(r *ConformanceReport) {
			r.Profiles[0].Tests[0].Outcome = Unknown
			r.Profiles[0].Tests[0].Message = "access denied"
			finish(r)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := completeReport(t)
			tc.modify(&r)
			if err := ValidateReport(r, true); err == nil {
				t.Fatal("invalid submission accepted")
			}
		})
	}
	r := completeReport(t)
	r.Profiles[0].Tests[0].Outcome = Skipped
	r.Profiles[0].Tests[0].Message = "unverified"
	finish(&r)
	if r.Result != Partial {
		t.Fatal(r.Result)
	}
	if err := ValidateReport(r, true); err != nil {
		t.Fatal(err)
	}
}

func TestUnitMatrixComparabilityAndPartial(t *testing.T) {
	root := t.TempDir()
	first := completeReport(t)
	second := completeReport(t)
	second.Implementation.Project = testOther
	second.Suite.Revision = strings.Repeat("c", 40)
	second.Profiles[0].Tests[0].Outcome = Skipped
	second.Profiles[0].Tests[0].Message = "not executed"
	finish(&second)
	for name, r := range map[string]ConformanceReport{"first": first, "second": second} {
		if err := WriteReports(filepath.Join(root, name), []string{ReportFormatYAML}, false, r); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := GenerateMatrix(root, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if strings.Count(s, "## multicluster.x-k8s.io/v1alpha2") != 2 || !strings.Contains(s, "| partial |") ||
		!strings.Contains(s, "| success |") ||
		!strings.Contains(s, "first/report.yaml") {
		t.Fatalf("unexpected matrix:\n%s", s)
	}
	second.Profiles[0].Tests[0].Outcome = Failed
	second.Profiles[0].Tests[0].Message = "violated"
	finish(&second)
	if err := WriteReports(filepath.Join(root, "second"), []string{ReportFormatYAML}, false, second); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := GenerateMatrix(root, &out); err == nil || out.Len() != 0 {
		t.Fatal("invalid matrix produced partial output")
	}
}

func TestUnitReadReportRejectsAmbiguousDocuments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.yaml")
	for _, data := range []string{
		"unknownField: true\n",
		"apiVersion: one\napiVersion: two\n",
		"apiVersion: one\n---\napiVersion: two\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadReport(path); err == nil {
			t.Fatalf("accepted ambiguous report %q", data)
		}
	}
}
