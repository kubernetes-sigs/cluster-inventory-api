# Cluster Inventory API Conformance Suite

Conformance tests for the Cluster Inventory APIs defined by
[KEP-4322](https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/4322-cluster-inventory),
tracked in [issue #60](https://github.com/kubernetes-sigs/cluster-inventory-api/issues/60).

The suite follows the model of the
[MCS-API conformance suite](https://github.com/kubernetes-sigs/mcs-api/tree/master/conformance):
a Ginkgo v2 test binary that runs against a cluster serving the APIs, tags
specs as `Required` (KEP MUSTs) or `Optional` (KEP SHOULDs and MAYs), links each spec
to the section of KEP-4322 it verifies, and generates a conformance report.

The framework is not tied to one API: each API is a separate section of the
suite and of the report, can be selected on its own, and can be reported in
its own file. The ClusterProfile API (aimed at cluster managers) is covered
today; the PlacementDecision API (aimed at consumers) and the Placement API,
once it lands, plug in the same way (see [Adding an API](#adding-an-api)).

## What the suite verifies

### ClusterProfile

ClusterProfile objects are created and reconciled by a cluster manager
implementation to represent registered member clusters - they are not input
resources created by API consumers. Exercising create/update/delete from a
test would verify the CRD and the Kubernetes API server rather than an
implementation, so the suite does not do that: CRD schema validation is
covered by this repository's envtest-based integration tests
(`test/integration`).

Instead, the suite **observes the implementation-managed ClusterProfile
objects of one cluster inventory** - per KEP-4322, the inventory is defined by
a namespace - and verifies the conventions the KEP places on cluster
managers:

- the `clusterprofiles` resource is served in `multicluster.x-k8s.io/v1alpha2`
  and is namespace scoped,
- registered member clusters are represented by ClusterProfile objects in the
  inventory namespace (exactly as many as a positive `--expected-clusters`,
  at least one otherwise),
- the `x-k8s.io/cluster-manager` label matches `spec.clusterManager.name`
  when present (Required), and its presence is reported separately (Optional;
  skipped if any managed profile omits it),
- the `multicluster.x-k8s.io/inventory-member-id` label is never empty when
  set (Required), is set on managed profiles (Optional), and identifies at
  most one ClusterProfile per member cluster within the inventory (Optional),
- the `ControlPlaneHealthy` and `Joined` conditions and the member cluster's
  Kubernetes version are reported (Optional).

The inventory-wide uniqueness check reports duplicate non-empty member IDs
even if other profiles lack an ID. If no duplicate is found, it is skipped
when the inventory is empty or any profile lacks a non-empty member ID:
there is not enough information to determine whether each member cluster has
at most one ClusterProfile. The skip reason is included in the HTML and YAML
reports.

## Running against an implementation

Register at least one cluster with the cluster manager under test using its
own registration mechanism, then point the suite at the resulting inventory:

```sh
make test-conformance CONFORMANCE_ARGS="--kubeconfig $HOME/.kube/config \
    --namespace fleet-system --cluster-manager my-cluster-manager"
```

Or directly:

```sh
cd conformance
go test -v ./... -args --kubeconfig $HOME/.kube/config \
    --namespace fleet-system --cluster-manager my-cluster-manager
```

The suite only needs permission to list ClusterProfile objects in the
inventory namespace, so a namespaced Role is sufficient: it reads the
namespace object to fail early on a wrong `--namespace`, but tolerates being
forbidden to.

The suite accepts the following flags:

| Flag | Description |
| ---- | ----------- |
| `--kubeconfig` | Path to the kubeconfig of the cluster serving the Cluster Inventory APIs. Standard loading rules (`$KUBECONFIG`, `~/.kube/config`) apply if unset. Prefer an absolute path when running through `make`: the suite executes from the `conformance/` directory, so relative paths resolve against it. |
| `--context` | Kubeconfig context to use. Defaults to the current context. |
| `--namespace` | **Required.** The inventory namespace containing the ClusterProfile objects managed by the implementation under test. The suite only reads from it. |
| `--cluster-manager` | Name of the cluster manager under test, as reported in `spec.clusterManager.name`. If unset, all ClusterProfile objects in the inventory namespace are verified. |
| `--expected-clusters` | Number of member clusters registered with the cluster manager under test. When positive, the suite verifies that exactly that many ClusterProfile objects exist in the inventory namespace; `0` (the default) leaves the count unknown and the suite only verifies that at least one does, since a conformance run needs at least one registered cluster to observe. |
| `--apis` | Comma-separated list of the APIs to test (`ClusterProfile`). Defaults to all APIs. |
| `--report-format` | Comma-separated list of the report formats to write: `html`, `yaml`. Defaults to `html`. |
| `--report-output` | `single` (default) writes one report covering all tested APIs; `per-api` writes one report per API. |
| `--organization` | Name of the organization responsible for the implementation being tested. |
| `--project` | Name of the implementation project being tested. |
| `--version` | Version of the implementation being tested. |
| `--url` | URL pointing to the implementation project or its documentation. |

## Reports

By default each run writes a single HTML report, `report.html`, covering all
APIs, into the working directory (this directory when run via `make` or
`go test`). The report options select what is written:

```sh
# HTML and YAML, one file per API: report-clusterprofile.html, report-clusterprofile.yaml
make test-conformance CONFORMANCE_ARGS="... --report-format html,yaml --report-output per-api"

# only the ClusterProfile API, as a single YAML report
make test-conformance CONFORMANCE_ARGS="... --apis ClusterProfile --report-format yaml"
```

Single reports are named `report.<format>`, per-API reports
`report-<api>.<format>` (lower-case API name); all of them are gitignored.
Each report lists the tests of every reported API grouped by `Required` and
`Optional`, with the totals per API and overall, and records the
implementation metadata flags. An unknown API name in `--apis` fails the run
before any spec is executed. `--apis` is applied as a Ginkgo label filter and
combines with a `--ginkgo.label-filter` you pass yourself, e.g.
`--ginkgo.label-filter=Required` to run only the Required specs.

## Exercising the suite without an implementation

Because the suite only observes, something has to play the cluster manager.
`hack/conformance-seed` populates an inventory namespace with sample
ClusterProfile objects shaped like those of a conformant implementation; the
flows below use it so the suite itself can be exercised in CI and during
development. They validate the suite and the CRDs - not a real
implementation.

`make test-conformance-envtest` runs everything against a temporary
[envtest](https://book.kubebuilder.io/reference/envtest) control plane with
this repository's CRDs installed: it downloads the envtest binaries, starts
the control plane, seeds the sample inventory, runs the suite, and tears
everything down again:

```sh
make test-conformance-envtest
```

The `Conformance Suite` CI job runs this flow, together with the module's
unit tests and lint, on every push and pull request.

`make test-conformance-kind` does the same against a kind cluster named
`cluster-inventory-conformance` (creating it if needed and installing the
CRDs). Docker (or another kind-supported provider) and `kubectl` must be
available; everything else is fetched automatically. Note that creating the
cluster switches your current kubectl context, as usual with kind.

```sh
make test-conformance-kind

# pin the node image / Kubernetes version:
make test-conformance-kind KIND_NODE_IMAGE=kindest/node:v1.35.0

# tear the cluster down when done:
make kind-conformance-down
```

Both flows use the `CONFORMANCE_NAMESPACE` (default `conformance-inventory`)
and `CONFORMANCE_MANAGER` (default `conformance-sample-manager`) make
variables for the seeded inventory, and pass `CONFORMANCE_ARGS` through, so
the report options above apply to them as well.

The control plane helper can also be run standalone to keep a cluster around
for iterating. From the repository root:

```sh
make envtest
KUBEBUILDER_ASSETS="$(bin/setup-envtest-release-0.23 use 1.35.0 --bin-dir bin -p path)" \
    go run ./hack/conformance-envtest --kubeconfig-out envtest.kubeconfig &
go run ./hack/conformance-seed --kubeconfig $PWD/envtest.kubeconfig
make test-conformance CONFORMANCE_ARGS="--kubeconfig $PWD/envtest.kubeconfig \
    --namespace conformance-inventory --cluster-manager conformance-sample-manager"
kill %1
```

## Importing the suite

The specs and the `TestConformance` entry point live in non-test files so
implementations can embed the suite in their own pipelines, mirroring how
downstreams consume the MCS-API suite:

```go
package conformance_test

import (
	"testing"

	"sigs.k8s.io/cluster-inventory-api/conformance"
)

func TestConformance(t *testing.T) {
	conformance.TestConformance(t)
}
```

The suite registers its flags on the standard `flag` package command line, so
the flags above are available to the importing test binary as well.

The module requires the version of `sigs.k8s.io/cluster-inventory-api` that
provides the `v1alpha2` API it tests: a pseudo-version of `main` until a
release includes it, after which the requirement is bumped to that release.
The `replace` directive in `go.mod` only takes effect inside this repository,
so importers resolve that requirement through the module proxy.

## Adding an API

Each API is identified by an `api` value in `conformance_suite.go`; its name
is at once the Ginkgo label of the API's specs, the value `--apis` accepts,
and the heading of its report section. To cover a new API:

1. Add an `api` value for it and append it to the `apis` registry, which also
   fixes the order of the report sections.
2. Put its specs in `ginkgo.Describe` containers labeled with the API's name
   (`ginkgo.Label(<api>.Name)`), written with `SpecifyWithSpecRef` so each
   spec links to the KEP section it verifies, and labeled `Required` or
   `Optional` (`RequiredLabel`/`OptionalLabel`).

The report generation checks that every spec carries the label of a known
API, so a container that misses the label fails the run.

## Scope and roadmap

The KEP does not yet standardize a registration input, so the suite treats
registration as an implementation-specific precondition and does not cover
registration/deregistration lifecycle (for example, deletion recovery).
Consumer-side conformance - inventory discovery and watching without merging
namespaces, deduplication by `multicluster.x-k8s.io/inventory-member-id`, and
authenticating to member clusters via `status.accessProviders` - as well as
properties propagated from member-cluster `ClusterProperty` resources (which
require member cluster access to verify), and the PlacementDecision API
specs, are tracked as follow-ups in
[issue #60](https://github.com/kubernetes-sigs/cluster-inventory-api/issues/60).
