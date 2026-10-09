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
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func validateMetadata(r ConformanceReport) error {
	var errs []error
	required := map[string]string{
		"implementation.organization": r.Implementation.Organization, "implementation.project": r.Implementation.Project,
		"implementation.version": r.Implementation.Version,
		"implementation.mode":    r.Implementation.Mode,
		"suite.version":          r.Suite.Version,
	}
	keys := make([]string, 0, len(required))
	for key := range required {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if strings.TrimSpace(required[key]) == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
	}
	for _, source := range []struct {
		name  string
		value Source
	}{{"implementation", r.Implementation.Source}, {"suite", r.Suite.Source}} {
		if !commitPattern.MatchString(source.value.Revision) {
			errs = append(errs, fmt.Errorf("%s.revision must be a full source commit", source.name))
		}
		if source.value.Dirty {
			errs = append(errs, fmt.Errorf("%s source must be clean for submission", source.name))
		}
	}
	if !validURL(r.Implementation.URL) {
		errs = append(errs, errors.New("implementation.url must be an absolute HTTP(S) URL"))
	}
	if !validURL(r.Implementation.Reproduction) {
		errs = append(
			errs,
			errors.New(
				"implementation.reproduction must identify a reproducible source artifact with an absolute HTTP(S) URL",
			),
		)
	}
	if len(r.Implementation.Contact) == 0 {
		errs = append(errs, errors.New("implementation.contact is required"))
	}
	for _, contact := range r.Implementation.Contact {
		if strings.TrimSpace(contact) == "" {
			errs = append(errs, errors.New("implementation.contact entries must not be empty"))
		}
	}
	return errors.Join(errs...)
}

// ValidateReport verifies the complete catalog and recomputes the result. A
// submission is tied to this catalog; use the matching suite for other catalogs.
func ValidateReport(r ConformanceReport, submission bool) error {
	if r.APIVersion != ReportAPIVersion || r.Kind != ReportKind {
		return fmt.Errorf("unsupported report schema %s/%s", r.APIVersion, r.Kind)
	}
	if _, err := time.Parse(time.RFC3339Nano, r.Date); err != nil {
		return fmt.Errorf("invalid report date: %w", err)
	}
	if r.RunMode != Development && r.RunMode != Submission {
		return fmt.Errorf("invalid run mode %q", r.RunMode)
	}
	if r.Specification.Revision != SpecRevision || r.Specification.URL != specRef("") ||
		r.Suite.CatalogDigest != CatalogDigest() {
		return errors.New("unknown specification or check catalog; validate with the matching suite revision")
	}
	if r.API.GroupVersion != "multicluster.x-k8s.io/v1alpha2" || r.API.Revision != APIRevision {
		return errors.New("unknown tested API revision")
	}
	if len(r.Profiles) != 1 {
		return errors.New("report must contain the selected producer profile exactly once")
	}
	if err := validateOptions(defaults(Options{
		Namespace:         r.Environment.InventoryNamespace,
		ExpectedClusters:  r.Environment.ExpectedClusters,
		SupportedFeatures: r.Profiles[0].SupportedFeatures,
	})); err != nil {
		return err
	}
	if err := validateProfile(r.Profiles[0]); err != nil {
		return err
	}
	expected := r.Profiles[0].Result
	if len(r.Errors) > 0 {
		expected = combine(expected, Undetermined)
	}
	if r.Result != expected {
		return errors.New("overall result does not match profile results and errors")
	}

	if submission {
		return validateSubmission(r)
	}
	return nil
}

func validateSubmission(r ConformanceReport) error {

	if r.RunMode != Submission {
		return errors.New("development reports cannot be submitted")
	}
	if err := validateMetadata(r); err != nil {
		return err
	}
	if r.Environment.KubernetesVersion == "" {
		return errors.New("observed Kubernetes version is required for submission")
	}
	if r.Result != Success && r.Result != Partial {
		return fmt.Errorf("%s reports cannot be accepted as conformance submissions", r.Result)
	}
	if len(r.Environment.ObservedProfiles) == 0 {
		return errors.New("submission must identify observed profiles")
	}
	objects := map[string]bool{}
	for _, object := range r.Environment.ObservedProfiles {
		key := object.Namespace + "/" + object.Name
		if object.Namespace != r.Environment.InventoryNamespace || object.Name == "" || object.UID == "" ||
			object.ResourceVersion == "" ||
			objects[key] {
			return fmt.Errorf("invalid observed profile %q", key)
		}
		objects[key] = true
	}

	return nil
}

func validateProfile(p ProfileReport) error {
	if p.Name != ProducerProfile || p.API != ClusterProfileAPI || p.Role != RoleProducer {
		return errors.New("unknown profile, API, or role")
	}
	o := Options{Namespace: "selected-inventory", SupportedFeatures: p.SupportedFeatures, RunMode: Development}
	if err := validateOptions(defaults(o)); err != nil {
		return err
	}
	if len(p.Tests) != len(catalog) {
		return fmt.Errorf("incomplete catalog: expected %d checks, got %d", len(catalog), len(p.Tests))
	}
	seen := map[string]bool{}
	for _, t := range p.Tests {
		if seen[t.ID] {
			return fmt.Errorf("duplicate check %q", t.ID)
		}
		seen[t.ID] = true
		idx := slices.IndexFunc(catalog, func(req Requirement) bool { return req.ID == t.ID })
		if idx < 0 || t.Requirement != catalog[idx] {
			return fmt.Errorf("unknown or modified requirement %q", t.ID)
		}
		switch t.Outcome {
		case Passed, Failed, Skipped, Unknown, NotSupported:
		default:
			return fmt.Errorf("invalid outcome %q for %s", t.Outcome, t.ID)
		}
		if t.Outcome != Passed && strings.TrimSpace(t.Message) == "" {
			return fmt.Errorf("%s requires a reason for %s", t.ID, t.Outcome)
		}
		if t.Outcome == NotSupported && t.Feature == "" {
			return fmt.Errorf("required check %s cannot be not-supported", t.ID)
		}
	}
	stats, result := summarize(p.Tests, p.SupportedFeatures)
	if stats != p.Statistics || result != p.Result {
		return errors.New("profile statistics or result does not match its checks")
	}
	return nil
}

func ReadReport(path string) (ConformanceReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConformanceReport{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var report ConformanceReport
	if err = decoder.Decode(&report); err != nil {
		return report, fmt.Errorf("decode %s: %w", path, err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return report, fmt.Errorf("%s must contain exactly one report document", path)
	}
	return report, nil
}

//go:embed report_template.gohtml
var reportHTML string

var htmlReport = template.Must(template.New("report").Parse(reportHTML))

// WriteReports atomically replaces each file after rendering is complete.
func WriteReports(dir string, formats []string, perAPI bool, r ConformanceReport) error {
	if len(formats) == 0 {
		return errors.New("at least one report format is required")
	}
	for _, format := range formats {
		if format != ReportFormatHTML && format != ReportFormatYAML {
			return fmt.Errorf("unknown report format %q", format)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, format := range slices.Compact(slices.Clone(formats)) {
		var data bytes.Buffer
		if format == ReportFormatHTML {
			if err := htmlReport.Execute(&data, r); err != nil {
				return err
			}
		} else {
			encoder := yaml.NewEncoder(&data)
			encoder.SetIndent(2)
			if err := encoder.Encode(r); err != nil {
				return err
			}
			if err := encoder.Close(); err != nil {
				return err
			}
		}
		base := "report"
		if perAPI {
			base += "-clusterprofile"
		}
		if err := atomicWrite(filepath.Join(dir, base+"."+format), data.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, data []byte) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".conformance-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer func() {
		removeErr := os.Remove(temp)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, removeErr)
		}
	}()
	if _, err = file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err = file.Chmod(0644); err != nil {
		return errors.Join(err, file.Close())
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

type matrixEntry struct {
	report ConformanceReport
	path   string
}

func escapeCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ", "<", "&lt;", ">", "&gt;").Replace(s)
}

// GenerateMatrix includes every accepted report, grouped by the exact tested
// suite/specification/API tuple; partial results never appear as conformant.
func GenerateMatrix(reportsDirectory string, w io.Writer) error {
	entries := []matrixEntry{}
	err := filepath.WalkDir(reportsDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		r, err := ReadReport(path)
		if err != nil {
			return err
		}
		if err = ValidateReport(r, true); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		relative, err := filepath.Rel(reportsDirectory, path)
		if err != nil {
			return err
		}
		entries = append(entries, matrixEntry{report: r, path: filepath.ToSlash(relative)})
		return nil
	})
	if err != nil {
		return err
	}
	key := func(e matrixEntry) string {
		r := e.report
		return strings.Join(
			[]string{
				r.Specification.Revision,
				r.Suite.Revision,
				r.API.Revision,
				r.Implementation.Organization,
				r.Implementation.Project,
				r.Implementation.Version,
				r.Implementation.Mode,
				r.Date,
				e.path,
			},
			"\x00",
		)
	}
	slices.SortFunc(entries, func(a, b matrixEntry) int { return strings.Compare(key(a), key(b)) })
	var out bytes.Buffer
	out.WriteString(
		"# Cluster Inventory API conformance\n\nEach row is a submitted run. " +
			"Only success denotes conformance for the required checks and claimed features. " +
			"Unclaimed recommendations remain visible in the linked report.\n\n",
	)
	if len(entries) == 0 {
		out.WriteString("No accepted reports.\n")
	}
	group := ""
	for _, e := range entries {
		r := e.report
		g := r.Specification.Revision + "/" + r.Suite.Revision + "/" + r.API.Revision
		if g != group {
			group = g
			fmt.Fprintf(
				&out,
				"## %s\n\nSpecification: `%s` · Suite: `%s` · API types: `%s`\n\n",
				r.API.GroupVersion,
				r.Specification.Revision,
				r.Suite.Revision,
				r.API.Revision,
			)
			out.WriteString("| Implementation | Version / commit | Mode | Profile | Result | Inventory | Run date |")
			for _, f := range features {
				fmt.Fprintf(&out, " %s |", f)
			}
			out.WriteString(" Report |\n|---|---|---|---|---|---|---|")
			for range features {
				out.WriteString("---|")
			}
			out.WriteString("---|\n")
		}
		p := r.Profiles[0]
		fmt.Fprintf(
			&out,
			"| %s / %s | %s / `%s` | %s | %s | %s | %s | %s |",
			escapeCell(r.Implementation.Organization),
			escapeCell(r.Implementation.Project),
			escapeCell(r.Implementation.Version),
			r.Implementation.Revision,
			escapeCell(r.Implementation.Mode),
			p.Name,
			p.Result,
			escapeCell(r.Environment.InventoryNamespace),
			r.Date,
		)
		for _, f := range features {
			value := "Unclaimed"
			if slices.Contains(p.SupportedFeatures, f) {
				value = "passed"
				for _, t := range p.Tests {
					if t.Feature == f && t.Outcome != Passed {
						value = string(t.Outcome)
						break
					}
				}
			}
			fmt.Fprintf(&out, " %s |", value)
		}
		u := url.URL{Path: e.path}
		fmt.Fprintf(&out, " [YAML](%s) |\n", u.String())
	}
	_, err = w.Write(out.Bytes())
	return err
}
