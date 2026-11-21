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
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cp "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
	cpclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

const (
	LevelMust             = "MUST"
	LevelShould           = "SHOULD"
	LevelMay              = "MAY"
	LevelInformative      = "Informative"
	ScopeAPI              = "api"
	ScopeManager          = "manager"
	ScopeInventory        = "inventory"
	ScopeDecision         = "decision"
	RoleProducer          = "producer"
	ReportFormatHTML      = "html"
	ReportFormatYAML      = "yaml"
	CheckAPIAvailability  = "ClusterProfileAPIAvailability"
	CheckNamespaced       = "ClusterProfileNamespaced"
	CheckRepresentation   = "RegisteredClusterRepresentation"
	CheckManagerValue     = "ClusterManagerLabelValue"
	CheckMemberIDValue    = "InventoryMemberIDValue"
	CheckManagerPresence  = "ClusterManagerLabelPresence"
	CheckMemberIDPresence = "InventoryMemberIDPresence"
	CheckHealthyPresence  = "ControlPlaneHealthyPresence"
	CheckJoinedPresence   = "JoinedPresence"
	CheckVersionPresence  = "KubernetesVersionPresence"
	ReportAPIVersion      = "conformance.multicluster.x-k8s.io/v1alpha1"
	ReportKind            = "ConformanceReport"
	ProducerProfile       = "CLUSTERPROFILE-PRODUCER"
	ClusterProfileAPI     = cp.ClusterProfileKind
	SpecRevision          = "3300d87b960c22051c66f1d1072e424a5cda5b5d"
	APIRevision           = "dc460e8ca90ddc100fb5c7ba3ad082f11b200537"
	Development           = "development"
	Submission            = "submission"
	RequiredLabel         = "Required"
	OptionalLabel         = "Optional"
)

// PlacementDecision producer profile, pinned to the KEP-5313 revision its catalog follows.
const (
	PlacementDecisionProducerProfile      = "PLACEMENTDECISION-PRODUCER"
	PlacementDecisionAPI                  = cp.PlacementDecisionKind
	PlacementDecisionSpecRevision         = "a6d15526d6383230e2d17cd4af9f7e783b581cbb"
	CheckPlacementDecisionAPIAvailability = "PlacementDecisionAPIAvailability"
	CheckPlacementDecisionNamespaced      = "PlacementDecisionNamespaced"
	CheckReferenceResolution              = "ClusterProfileReferenceResolution"
	CheckReferenceUniqueness              = "ClusterProfileReferenceUniqueness"
	CheckDecisionKeyCorrelation           = "DecisionKeyCorrelation"
	CheckDecisionIndexOrdering            = "DecisionIndexOrdering"
	CheckPlacementKeyPresence             = "PlacementKeyPresence"
	CheckSchedulerNamePresence            = "SchedulerNamePresence"
	CheckDecisionReasonPresence           = "DecisionReasonPresence"
	CheckSliceNameMatchesIndex            = "SliceNameMatchesIndex"
)

// Outcome records whether a requirement was actually verified.
type Outcome string

const (
	Passed       Outcome = "passed"
	Failed       Outcome = "failed"
	Skipped      Outcome = "skipped"
	Unknown      Outcome = "unknown"
	NotSupported Outcome = "not-supported"
)

type Result string

const (
	Success      Result = "success"
	Failure      Result = "failure"
	Partial      Result = "partial"
	Undetermined Result = "unknown"
)

type Source struct {
	Version  string `json:"version"  yaml:"version"`
	Revision string `json:"revision" yaml:"revision"`
	Dirty    bool   `json:"dirty"    yaml:"dirty"`
}

type Implementation struct {
	Organization string `json:"organization" yaml:"organization"`
	Project      string `json:"project"      yaml:"project"`
	Source       `         json:",inline"      yaml:",inline"`
	Mode         string   `json:"mode"         yaml:"mode"`
	URL          string   `json:"url"          yaml:"url"`
	Contact      []string `json:"contact"      yaml:"contact"`
	// Reproduction points to a pinned test entry point or a reproducible source artifact.
	Reproduction string `json:"reproduction" yaml:"reproduction"`
}

type Specification struct {
	Revision string `json:"revision" yaml:"revision"`
	URL      string `json:"url"      yaml:"url"`
}

type SuiteInfo struct {
	Source        `       json:",inline"       yaml:",inline"`
	CatalogDigest string `json:"catalogDigest" yaml:"catalogDigest"`
}

type APIInfo struct {
	GroupVersion string `json:"groupVersion" yaml:"groupVersion"`
	// Revision identifies the API types tested, not an inferred installed CRD release.
	Revision string `json:"revision"     yaml:"revision"`
}

type ObjectReference struct {
	Namespace       string `json:"namespace"       yaml:"namespace"`
	Name            string `json:"name"            yaml:"name"`
	UID             string `json:"uid"             yaml:"uid"`
	ResourceVersion string `json:"resourceVersion" yaml:"resourceVersion"`
}

type Environment struct {
	KubernetesVersion  string            `json:"kubernetesVersion"  yaml:"kubernetesVersion"`
	InventoryNamespace string            `json:"inventoryNamespace" yaml:"inventoryNamespace"`
	ClusterManager     string            `json:"clusterManager"     yaml:"clusterManager"`
	ExpectedClusters   int               `json:"expectedClusters"   yaml:"expectedClusters"`
	ObservedProfiles   []ObjectReference `json:"observedProfiles"   yaml:"observedProfiles"`
	// PlacementDecision producer inputs; omitted from ClusterProfile reports.
	DecisionNamespace string            `json:"decisionNamespace,omitempty" yaml:"decisionNamespace,omitempty"`
	SchedulerName     string            `json:"schedulerName,omitempty"     yaml:"schedulerName,omitempty"`
	ObservedDecisions []ObjectReference `json:"observedDecisions,omitempty" yaml:"observedDecisions,omitempty"`
}

type Requirement struct {
	ID          string `json:"id"                yaml:"id"`
	Description string `json:"description"       yaml:"description"`
	// Level preserves the specification's strength; Informative features are opt-in capabilities.
	Level   string `json:"level"             yaml:"level"`
	Feature string `json:"feature,omitempty" yaml:"feature,omitempty"`
	Scope   string `json:"scope"             yaml:"scope"`
	Ref     string `json:"ref"               yaml:"ref"`
}

// Optional reports whether the requirement applies only to implementations that claim its feature.
func (r Requirement) Optional() bool { return r.Feature != "" }

type TestResult struct {
	Requirement `        json:",inline"           yaml:",inline"`
	Outcome     Outcome `json:"outcome"           yaml:"outcome"`
	Message     string  `json:"message,omitempty" yaml:"message,omitempty"`
}

type Statistics struct {
	Passed       int `json:"passed"       yaml:"passed"`
	Failed       int `json:"failed"       yaml:"failed"`
	Skipped      int `json:"skipped"      yaml:"skipped"`
	Unknown      int `json:"unknown"      yaml:"unknown"`
	NotSupported int `json:"notSupported" yaml:"notSupported"`
	Total        int `json:"total"        yaml:"total"`
}

type ProfileReport struct {
	Name              string       `json:"name"              yaml:"name"`
	API               string       `json:"api"               yaml:"api"`
	Role              string       `json:"role"              yaml:"role"`
	SupportedFeatures []string     `json:"supportedFeatures" yaml:"supportedFeatures"`
	Result            Result       `json:"result"            yaml:"result"`
	Statistics        Statistics   `json:"statistics"        yaml:"statistics"`
	Tests             []TestResult `json:"tests"             yaml:"tests"`
}

type ConformanceReport struct {
	APIVersion     string          `json:"apiVersion"       yaml:"apiVersion"`
	Kind           string          `json:"kind"             yaml:"kind"`
	Date           string          `json:"date"             yaml:"date"`
	RunMode        string          `json:"runMode"          yaml:"runMode"`
	Implementation Implementation  `json:"implementation"   yaml:"implementation"`
	Suite          SuiteInfo       `json:"suite"            yaml:"suite"`
	Specification  Specification   `json:"specification"    yaml:"specification"`
	API            APIInfo         `json:"api"              yaml:"api"`
	Environment    Environment     `json:"environment"      yaml:"environment"`
	Profiles       []ProfileReport `json:"profiles"         yaml:"profiles"`
	Result         Result          `json:"result"           yaml:"result"`
	Errors         []string        `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// Options scopes an observation run. Injected clients allow downstream runners
// to reuse their connection; neither client is used to mutate resources.
// Each selected profile gets its own report, so selecting more than one
// requires ReportPerAPI.
type Options struct {
	Kubeconfig        string
	Context           string
	Config            *rest.Config
	Namespace         string
	ClusterManager    string
	ExpectedClusters  int
	DecisionNamespace string
	SchedulerName     string
	APIs              []string
	Profiles          []string
	SupportedFeatures []string
	TestFilter        string
	RunMode           string
	Implementation    Implementation
	Suite             Source
	ReportDirectory   string
	ReportFormats     []string
	ReportPerAPI      bool
	Timeout           time.Duration
	Kubernetes        kubernetes.Interface
	ClusterInventory  cpclient.Interface
}

// profile holds what differs between conformance profiles. The runner, report
// writer, and validator are shared, so supporting another API adds one entry.
type profile struct {
	name     string
	api      string
	spec     Specification
	catalog  []Requirement
	features []string
	digest   func() string
	// environment records the profile's inputs. validateEnvironment checks them
	// the same way for a run's Options and for a stored report.
	environment         func(Options) Environment
	validateEnvironment func(Environment) error
	// observed names the kind, namespace, and objects a submission must identify.
	observed func(Environment) (kind, namespace string, objects []ObjectReference)
	// namespaceColumn heads the matrix column showing that namespace.
	namespaceColumn string
	// observe reads the implementation's output once and returns the check for each requirement.
	observe func(context.Context, Options, *ConformanceReport) (func(Requirement) TestResult, error)
}
