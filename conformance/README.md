# Conformance

Cluster manager developers use this suite to test the ClusterProfile objects
produced by their implementation. The suite observes an inventory on a running
cluster and generates HTML and YAML reports. Each report identifies the
implementation, suite, specification, and API revisions used for that run.

## Table of Contents

- [Conformance Tests](#conformance-tests)
  - [Running Tests](#running-tests)
  - [Integrating the Suite](#integrating-the-suite)
  - [Interpreting Results](#interpreting-results)
  - [Compiling Conformance tests](#compiling-conformance-tests)
  - [Conformance Profiles](#conformance-profiles)
- [Conformance Reports](#conformance-reports)
- [Contributing to Conformance](#contributing-to-conformance)

## Conformance Tests

The `CLUSTERPROFILE-PRODUCER` profile covers the `ClusterProfile` API in
`multicluster.x-k8s.io/v1alpha2`. Its requirement catalog references a fixed
[KEP-4322 revision](https://github.com/kubernetes/enhancements/blob/3300d87b960c22051c66f1d1072e424a5cda5b5d/keps/sig-multicluster/4322-cluster-inventory/README.md).
Required checks cover KEP MUST requirements; Optional checks cover capabilities an
implementation can claim through `--supported-features`.

| Checks | Objects inspected | Requirement |
| --- | --- | --- |
| API availability and namespace scope | API discovery and access to the selected inventory | Required |
| Registered-cluster representation | Profiles selected by `spec.clusterManager.name` | At least one; an exact count when supplied by the runner |
| Cluster manager label value | Selected profiles carrying `x-k8s.io/cluster-manager` | Equals `spec.clusterManager.name` when present |
| Inventory member ID value | Selected profiles carrying `multicluster.x-k8s.io/inventory-member-id` | Non-empty when present |
| Member uniqueness | All profiles in the namespace, including other managers | No repeated non-empty ID; missing IDs leave uniqueness unverified |
| Optional capabilities | Selected profiles | Reported individually; required for success when claimed |

The same member ID in another namespace belongs to another inventory and is
not a duplicate in this inventory. A duplicate is reported even when other
profiles omit IDs. The expected count does not establish a correspondence
between registered member clusters and individual profiles; the implementation's
test setup must establish that correspondence.

The suite checks a snapshot of output conventions. Condition presence does not
verify cluster health, and a non-empty version does not verify the member's
actual version. Registration lifecycle, consumer behavior, credentials, and
property propagation from member-cluster sources are outside this profile.
CRD schema validation is covered by this repository's integration tests.

### Running Tests

#### Prerequisites

- Use a checkout containing the `conformance` module and the Go version required
  by its `go.mod`. Record the suite revision for repeatable runs.
- Install the implementation and its ClusterProfile CRD on the inventory cluster.
- Register member clusters using the implementation's own mechanism, select the
  inventory to test, and wait for the implementation to publish their profiles.
- Provide a kubeconfig with permission to list all ClusterProfiles in that
  namespace and perform Kubernetes API discovery. The suite also reads the
  namespace to catch configuration errors, but tolerates a forbidden namespace
  read.

Cluster registration, inventory selection, and readiness are implementation
preconditions. The suite does not create, update, or delete ClusterProfiles and
does not operate on member clusters.

#### ClusterProfile Tests

The following example observes two registered members of `fleet-manager` in
`fleet-system`. Replace the kubeconfig, context, namespace, manager, count, and
claimed features with those of your implementation. Run from this repository's
root directory:

```sh
cd conformance
REPORT_DIR=$(mktemp -d)
go test -count=1 -run '^TestConformance$' -v . -args \
    --kubeconfig /absolute/path/hub.kubeconfig \
    --context inventory-hub \
    --namespace fleet-system \
    --cluster-manager fleet-manager \
    --expected-clusters 2 \
    --supported-features ClusterManagerLabel,InventoryMemberID,InventoryMemberUniqueness \
    --report-format html,yaml \
    --report-dir "$REPORT_DIR"
```

This is a development run. Open `$REPORT_DIR/report.html` to inspect the checks;
`$REPORT_DIR/report.yaml` contains the same results in machine-readable form.
`-count=1` disables Go test caching so each invocation observes the cluster again.
The root `make test-conformance` target also accepts observer flags through
`CONFORMANCE_ARGS`; relative paths passed to it resolve from `conformance/`.

#### Supported Features

Claim only capabilities the implementation supplies. Unclaimed Optional checks
remain visible in reports but do not affect the overall result.

| Feature | What the suite checks |
| --- | --- |
| `ClusterManagerLabel` | The cluster manager label is present on selected profiles |
| `InventoryMemberID` | Selected profiles provide a non-empty inventory member ID |
| `InventoryMemberUniqueness` | Non-empty member IDs occur at most once across the inventory; missing IDs leave the check incomplete |
| `ControlPlaneHealthy` | Selected profiles contain the `ControlPlaneHealthy` condition |
| `Joined` | Selected profiles contain the `Joined` condition |
| `KubernetesVersion` | Selected profiles report a non-empty Kubernetes version |

`InventoryMemberUniqueness` also requires claiming `InventoryMemberID`. Its check
covers all profiles in the namespace, including profiles belonging to other
cluster managers. Supplying `--cluster-manager` does not narrow that check.

#### Suite Level Options

| Option | Meaning |
| --- | --- |
| `--kubeconfig`, `--context` | Connection to the inventory cluster; standard Kubernetes loading rules apply when omitted |
| `--namespace` | Required inventory namespace |
| `--cluster-manager` | `spec.clusterManager.name` selection; empty inspects all managers |
| `--expected-clusters` | Zero requires at least one selected profile; a positive value requires exactly that number |
| `--apis`, `--profiles` | API/profile selection; currently `ClusterProfile` and `CLUSTERPROFILE-PRODUCER` |
| `--supported-features` | Comma-separated Optional capabilities claimed by the implementation |
| `--test-filter` | Development-only label expression using check IDs, API/profile names, Required, Optional, or features |
| `--run-mode` | `development` (default) or `submission` |
| `--timeout` | API observation deadline; defaults to 30 seconds |
| `--report-dir` | Output directory; defaults to the working directory |
| `--report-format` | `html`, `yaml`, or both; defaults to `html` |
| `--report-output` | `single` (default) or `per-api` |

Unknown APIs, profiles, or features fail the run. Use `--test-filter` to select
observer checks during development and Go test `-v` to control verbosity. Use
`go test -run '^TestConformance$' . -args -help` from `conformance/` for all flags,
including implementation and suite metadata.

### Integrating the Suite

The suite is a separate Go module. Implementations can import it into their own
Go tests or a separate test module to keep test dependencies outside the runtime
dependency graph.

Choose a publicly fetchable commit containing the conformance module and set
`SUITE_REVISION` to its full commit hash. From your test module, pin that revision:

```sh
go get sigs.k8s.io/cluster-inventory-api/conformance@"$SUITE_REVISION"
```

The API module and conformance module are versioned separately. An API release
alone does not identify the suite used to test an implementation. A local
workspace or replacement can be used for suite development; reports must identify
the actual suite source, as described in the [submission guide](reports/README.md).

Add the following entry point in `conformance_test.go`:

```go
package conformance_test

import (
	"testing"

	"sigs.k8s.io/cluster-inventory-api/conformance"
)

func TestClusterInventoryConformance(t *testing.T) {
	conformance.TestConformance(t)
}
```

Run this test with `go test -count=1 -run '^TestClusterInventoryConformance$' -v .`,
passing the observer flags after `-args` as in [Running Tests](#running-tests).
An implementation's CI can prepare the inventory, execute this test, and retain
the reports as artifacts.

For programmatic configuration, `RunConformance(t, options)` accepts per-run
`Options`, including connection settings or explicit clients. `Run(ctx, options)`
returns a typed report and an error without invoking Go test failure handlers.
`RegisterFlags` configures an independent `flag.FlagSet`. The `TestConformance`
entry point uses observer flags registered on `flag.CommandLine` at import time.

### Interpreting Results

Each check records one outcome:

| Outcome | Meaning |
| --- | --- |
| `passed` | The check verified its requirement, including satisfied conditional requirements |
| `failed` | The observed output violates the requirement or recommendation |
| `skipped` | The check was excluded or information needed to verify it is missing |
| `unknown` | An environment or access error prevented verification |
| `not-supported` | An Optional capability is not provided |

Required checks always affect the profile result. Optional checks affect it only
when their feature is claimed. Unclaimed SHOULD deviations remain visible, and
MAY or Informative capabilities can be omitted. Informative denotes an opt-in
capability rather than an additional normative requirement in the KEP.

| Overall result | Meaning |
| --- | --- |
| `success` | All Required checks and checks for claimed features passed |
| `partial` | At least one of those checks was skipped, without a failure or unknown result |
| `unknown` | At least one of those checks could not be determined, or a run error occurred, without a failure |
| `failure` | A Required check found a violation, or a claimed feature did not meet its requirement |

Precedence is `failure`, `unknown`, `partial`, then `success`. No selected checks
produces `unknown` and a non-zero exit. A Go test exit code alone is not a
conformance declaration; inspect the report's result and claimed features.
The runner writes the report even when a check fails or verification is
incomplete, and exits unsuccessfully for any result other than `success`.

For access or discovery errors, check the kubeconfig and permissions before
rerunning. If no selected profiles exist, verify registration, inventory
selection, and readiness in the implementation. When uniqueness is skipped,
inspect the profiles identified in its reason for missing member IDs.

### Compiling Conformance tests

A test binary can run on a machine that has access to the inventory cluster
without requiring Go on that machine. From the repository root, build it with:

```sh
CONFORMANCE_BIN_DIR=$(mktemp -d)
(cd conformance && go test -c -o "$CONFORMANCE_BIN_DIR/conformance.test" .)
```

Copy the binary to the test environment if needed. To build for a different
platform, set `GOOS` and `GOARCH` on the `go test -c` command. For example,
`GOOS=linux GOARCH=amd64` builds for Linux on AMD64.

On the execution machine, set `CONFORMANCE_BIN_DIR` to the directory containing
the binary if it differs from the build output directory. Execute it with Go
test flags followed by observer options:

```sh
"$CONFORMANCE_BIN_DIR/conformance.test" -test.run '^TestConformance$' -test.v \
    --kubeconfig /absolute/path/hub.kubeconfig \
    --context inventory-hub \
    --namespace fleet-system \
    --cluster-manager fleet-manager \
    --expected-clusters 2 \
    --supported-features ClusterManagerLabel,InventoryMemberID,InventoryMemberUniqueness \
    --report-format html,yaml \
    --report-dir "$CONFORMANCE_BIN_DIR"
```

Pass observer flags directly to the binary, without the `-args` separator used
by `go test`. Each binary execution observes the cluster again. Use the suite
revision from which the binary was built when supplying submission metadata.

### Conformance Profiles

A conformance profile groups the Required checks for an API and its Optional
capabilities. This suite currently provides `CLUSTERPROFILE-PRODUCER` for
`ClusterProfile`; both are selected by default. Specify them explicitly with
`--apis ClusterProfile --profiles CLUSTERPROFILE-PRODUCER`, and use
`--supported-features` to claim the capabilities of the implementation under
test. Profile selection does not replace the inventory and registration
prerequisites in [Running Tests](#running-tests).

## Conformance Reports

HTML and YAML are rendered from the same report. Single reports are named
`report.html` and `report.yaml`; per-API output uses
`report-clusterprofile.html` and `report-clusterprofile.yaml`.

YAML uses `conformance.multicluster.x-k8s.io/v1alpha1`, kind `ConformanceReport`.
This is a report schema, not a CRD to install; the tested API remains
`multicluster.x-k8s.io/v1alpha2`.

| Metadata | Interpretation |
| --- | --- |
| Implementation | Organization, project, version, source commit, source state, operating mode, URL, contacts, and reproduction reference |
| Suite | Version or source snapshot, source commit, source state, and requirement catalog digest |
| Specification | Fixed normative source revision and URL |
| API | Group version and API types revision used by the suite, not an inferred installed CRD release |
| Environment | Observed Kubernetes version, inventory, manager selection, expected count, and object UID/resourceVersion |
| Profiles | Claimed features, every catalog check, counts, and computed results |

Development reports can omit source metadata and contain filtered results. They
cannot be submitted as conformance results. To generate a submission report,
validate it, and open a results pull request, follow
[Conformance Reports](reports/README.md). That guide also defines the report
layout, reproduction requirements, and matrix generation commands.

## Contributing to Conformance

Conformance requirements and supported feature names are defined in
[catalog.go](catalog.go), with observation logic in [checks.go](checks.go).
Each requirement records its specification reference, level, and scope. Add
checks for requirements supported by the referenced specification and keep
implementation-specific setup in the implementation's test suite.

When changing a check, cover conforming and non-conforming observations, missing
prerequisites, and access errors in [suite_test.go](suite_test.go). Changes to
report acceptance or matrix generation belong in [report_test.go](report_test.go).
Keep the observer read-only and preserve the distinction between a failed check
and an environment that prevents verification.

From `conformance/`, run the module's unit tests:

```sh
go test -race -count=1 -run '^TestUnit' ./...
```

### Testing the Suite

From the repository root, `make test-conformance-envtest` exercises the observer
against a temporary control plane. `make test-conformance-kind` exercises it
against the dedicated `cluster-inventory-conformance` kind cluster; use
`make kind-conformance-down` to delete that cluster.

Both targets use `hack/conformance-seed` to create sample output. They test the
suite and CRDs, not a cluster manager's conformance. The existing CI job also runs
the module's unit tests and lint. Implementation conformance runs use the
implementation-managed inventory described in [Running Tests](#running-tests).
