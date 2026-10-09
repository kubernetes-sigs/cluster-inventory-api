# Cluster Inventory API conformance

Cluster manager developers use this module to check the ClusterProfile objects
that their implementation publishes. API consumers can use its reports to see
which output conventions an implementation meets, for a fixed specification,
API revision, suite revision, and implementation version.

The observer reads one inventory namespace. Cluster registration and inventory
selection belong to the implementation. The observer does not create, update,
or delete ClusterProfiles, and it does not operate on member clusters.

## Coverage

`CLUSTERPROFILE-PRODUCER` covers the `ClusterProfile` API in
`multicluster.x-k8s.io/v1alpha2`. The requirement catalog links to the fixed
[KEP-4322 revision](https://github.com/kubernetes/enhancements/blob/3300d87b960c22051c66f1d1072e424a5cda5b5d/keps/sig-multicluster/4322-cluster-inventory/README.md).

| Checks | Objects inspected | Acceptance |
| --- | --- | --- |
| API availability and namespace scope | API discovery and access to the selected inventory | Required |
| Registered-cluster representation | Profiles selected by `spec.clusterManager.name` | At least one; an exact count when supplied by the runner |
| Cluster manager label value | Selected profiles carrying `x-k8s.io/cluster-manager` | Equals `spec.clusterManager.name` |
| Inventory member ID value | Selected profiles carrying `multicluster.x-k8s.io/inventory-member-id` | Non-empty |
| Member uniqueness | All profiles in the namespace, including other managers | No repeated non-empty ID; missing IDs leave uniqueness unverified |
| Optional capabilities | Selected profiles | Reported individually; enforced when claimed |

The same member ID in another namespace belongs to another inventory and is
not a duplicate in this inventory. A duplicate is reported even when other
profiles omit IDs. The exact count does not establish a correspondence between
registered member clusters and individual profiles; that precondition belongs
to the implementation's runner.

These checks examine a snapshot of output conventions. Condition presence does
not verify cluster health, and a non-empty version does not verify the member's
actual version. Registration lifecycle, consumer behavior, credentials, and
property propagation from member-cluster sources are outside this profile.
CRD schema validation remains in the repository's integration tests.

## Implementation integration

The package offers an explicit runner with per-run options and no shared clients:

```go
func TestClusterInventory(t *testing.T) {
    conformance.RunConformance(t, conformance.Options{
        Namespace: "fleet-system",
        ClusterManager: "open-cluster-management",
        ExpectedClusters: 2,
        SupportedFeatures: conformance.SupportedFeatureNames(),
        ReportDirectory: os.Getenv("CONFORMANCE_REPORT_DIR"),
        ReportFormats: []string{"html", "yaml"},
    })
}
```

Only claim capabilities that the implementation supplies. The supported names
are `ClusterManagerLabel`, `InventoryMemberID`, `InventoryMemberUniqueness`,
`ControlPlaneHealthy`, `Joined`, and `KubernetesVersion`. Uniqueness support also
requires member ID support. Features outside this list cannot be claimed.

`Run(ctx, options)` returns a typed report and an error without invoking Go test
failure handlers. `RunConformance(t, options)` also writes reports and fails the
Go test when verification or writing fails. `RegisterFlags` can configure an
independent `flag.FlagSet`. The compatibility `TestConformance(t)` entry point
uses registered command-line flags; importing the package registers these flags
on `flag.CommandLine`, but registers no Ginkgo specs.

| Input | Meaning |
| --- | --- |
| `--kubeconfig`, `--context` | Standard Kubernetes loading rules apply when omitted |
| `--namespace` | Required inventory namespace |
| `--cluster-manager` | Optional `spec.clusterManager.name` selection; empty inspects all managers |
| `--expected-clusters` | Zero requires at least one profile; positive requires exactly that number |
| `--apis`, `--profiles` | API/profile selection; unknown names fail |
| `--supported-features` | Comma-separated optional capabilities claimed by the implementation |
| `--test-filter` | Development-only label expression using check IDs, API/profile names, Required, Optional, or features |
| `--run-mode` | `development` (default) or `submission` |
| `--timeout` | API observation deadline; defaults to 30 seconds |
| `--report-dir` | Output directory; defaults to the working directory |
| `--report-format` | `html`, `yaml`, or both; defaults to `html` |
| `--report-output` | `single` (default) or `per-api` |

`--ginkgo.label-filter` aliases `--test-filter`; `--ginkgo.v` is accepted for
compatibility. Go test `-v` controls verbosity. Connection permissions only need
ClusterProfile listing and Kubernetes discovery; reading the namespace is an
early configuration check, and a forbidden namespace read is tolerated.

## Result contract

| Individual outcome | Meaning |
| --- | --- |
| `passed` | The check verified its requirement, including satisfied conditional requirements |
| `failed` | The observed output violates this requirement or recommendation |
| `skipped` | The check was excluded or its prerequisite information is incomplete |
| `unknown` | An environment or access error prevented verification |
| `not-supported` | An optional capability is not provided |

Required checks always affect the profile result. Optional checks affect that
result only when their feature is claimed. Unclaimed SHOULD deviations remain
visible, and MAY or Informative capabilities can be omitted. Informative denotes
an opt-in capability rather than a new normative requirement in the KEP.

Overall precedence is `failure`, `unknown`, `partial`, then `success`: a required
or claimed feature violation produces failure; an undetermined gated check or a
run error produces unknown; an unexecuted gated check produces partial; otherwise
all gated checks must have passed. No selected checks produces unknown and a
non-zero exit. A Go test exit code alone is not a conformance declaration.

Submission runs prohibit check filters, require fixed source metadata, and verify
the generated report. Development reports can record incomplete metadata and
partial results; they are not accepted as submissions.

## Reports

The versioned YAML schema is `conformance.multicluster.x-k8s.io/v1alpha1`, kind
`ConformanceReport`. This is a report format, not a CRD to install. HTML and YAML
are rendered from the same report. Output names are `report.html`/`report.yaml`,
or `report-clusterprofile.html`/`report-clusterprofile.yaml` for per-API output.

| Metadata | Interpretation |
| --- | --- |
| Implementation organization, project, version, revision, dirty, mode, URL, contacts, reproduction | Identifies the implementation and an immutable test source or reproduction artifact |
| Suite version, revision, dirty, catalog digest | Identifies the actual suite and requirement catalog |
| Specification revision and URL | Fixed normative source |
| API group version and revision | API types used by the suite; not an inferred installed CRD release |
| Environment | Observed Kubernetes version, inventory, manager selection, expected count, and observed object UID/resourceVersion |
| Profiles | Claimed capabilities, every catalog check, counts, and the computed result |

Implementation flags are `--organization`, `--project`, `--version`, `--revision`,
`--dirty`, `--mode`, `--url`, `--contact`, and `--reproduction`. Suite flags are
`--suite-version`, `--suite-revision`, and `--suite-dirty`. Submission revisions
must be full commit hashes, and source trees must be clean. A local replacement's
revision must be supplied explicitly: the importing binary's commit identifies
the importer, not the shared suite.

The report CLI has two interfaces:

- `report validate <report.yaml>...` verifies submission acceptance and recomputes results.
- `report matrix --reports-dir <directory>` renders all accepted YAML reports as Markdown.

Matrix rows retain implementation versions, modes, inventories, and run dates.
They are grouped by the exact specification, suite, and tested API revisions.
Partial results remain marked partial; unclaimed features remain unclaimed.
Invalid input aborts generation before emitting a matrix.

Accepted report storage and invariants are defined in [reports](reports/README.md).

## Suite verification

The existing envtest and kind targets use `hack/conformance-seed` to provide
sample output. They test the observer and CRDs, not a cluster manager's conformance.
The separate module's unit tests exercise outcome classification, read-only
access, report integrity, and matrix generation. The existing CI job covers the
module and the seeded observer execution.
