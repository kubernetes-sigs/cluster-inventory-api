# ClusterProfile API Conformance Suite

Conformance tests for the [ClusterProfile API](https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/4322-cluster-inventory)
(KEP-4322), tracked in [issue #60](https://github.com/kubernetes-sigs/cluster-inventory-api/issues/60).

The suite follows the model of the
[MCS-API conformance suite](https://github.com/kubernetes-sigs/mcs-api/tree/master/conformance):
a Ginkgo v2 test binary that runs against a cluster serving the ClusterProfile
API, tags specs as `Required` (KEP MUSTs) or `Optional` (KEP SHOULDs), links
each spec to the section of KEP-4322 it verifies, and generates both a YAML
and an HTML conformance report.

## What the suite verifies

ClusterProfile objects are created and reconciled by a cluster manager
implementation to represent registered member clusters — they are not input
resources created by API consumers. Exercising create/update/delete from a
test would verify the CRD and the Kubernetes API server rather than an
implementation, so the suite does not do that: CRD schema validation is
covered by this repository's envtest-based integration tests
(`test/integration`).

Instead, the suite **observes the implementation-managed ClusterProfile
objects of one cluster inventory** — per KEP-4322, the inventory is defined by
a namespace — and verifies the conventions the KEP places on cluster
managers:

- the `clusterprofiles` resource is served in `multicluster.x-k8s.io/v1alpha2`
  and is namespace scoped,
- registered member clusters are represented by ClusterProfile objects in the
  inventory namespace,
- the `x-k8s.io/cluster-manager` label is present and matches
  `spec.clusterManager.name`,
- the `multicluster.x-k8s.io/inventory-member-id` label is never empty when
  set (Required), is set on managed profiles (Optional), and identifies at
  most one ClusterProfile per member cluster within the inventory (Optional),
- the `ControlPlaneHealthy` and `Joined` conditions and the member cluster's
  Kubernetes version are reported (Optional).

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

The suite accepts the following flags:

| Flag | Description |
| ---- | ----------- |
| `--kubeconfig` | Path to the kubeconfig of the cluster serving the ClusterProfile API. Standard loading rules (`$KUBECONFIG`, `~/.kube/config`) apply if unset. Prefer an absolute path when running through `make`: the suite executes from the `conformance/` directory, so relative paths resolve against it. |
| `--context` | Kubeconfig context to use. Defaults to the current context. |
| `--namespace` | **Required.** The inventory namespace containing the ClusterProfile objects managed by the implementation under test. The suite only reads from it. |
| `--cluster-manager` | Name of the cluster manager under test, as reported in `spec.clusterManager.name`. If unset, all ClusterProfile objects in the inventory namespace are verified. |
| `--organization` | Name of the organization responsible for the implementation being tested. |
| `--project` | Name of the implementation project being tested. |
| `--version` | Version of the implementation being tested. |
| `--url` | URL pointing to the implementation project or its documentation. |

Each run writes `report.yaml` and `report.html` into the working directory
(this directory when run via `make` or `go test`); both are gitignored. The
implementation metadata flags are recorded in the reports.

## Exercising the suite without an implementation

Because the suite only observes, something has to play the cluster manager.
`hack/conformance-seed` populates an inventory namespace with sample
ClusterProfile objects shaped like those of a conformant implementation; the
flows below use it so the suite itself can be exercised in CI and during
development. They validate the suite and the CRDs — not a real
implementation.

`make test-conformance-envtest` runs everything against a temporary
[envtest](https://book.kubebuilder.io/reference/envtest) control plane with
this repository's CRDs installed: it downloads the envtest binaries, starts
the control plane, seeds the sample inventory, runs the suite, and tears
everything down again:

```sh
make test-conformance-envtest
```

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
variables for the seeded inventory.

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

## Scope and roadmap

The KEP does not yet standardize a registration input, so the suite treats
registration as an implementation-specific precondition and does not cover
registration/deregistration lifecycle (for example, deletion recovery).
Consumer-side conformance — inventory discovery and watching without merging
namespaces, deduplication by `multicluster.x-k8s.io/inventory-member-id`, and
authenticating to member clusters via `status.accessProviders` — as well as
properties propagated from member-cluster `ClusterProperty` resources (which
require member cluster access to verify) are tracked as follow-ups in
[issue #60](https://github.com/kubernetes-sigs/cluster-inventory-api/issues/60).
