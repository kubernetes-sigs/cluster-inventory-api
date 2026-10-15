# Conformance Reports

Cluster manager maintainers submit conformance results by opening a pull request
to this repository. Each YAML report describes one implementation version and
operating mode, observing one inventory with a fixed suite and specification.
The suite writes reports locally; it does not upload them or open pull requests.

See [Conformance Tests](../README.md) for prerequisites, test coverage, and
implementation integration. This guide covers generating a submission report,
validating it, and preparing the files for review.

## How this folder is structured

Reports are grouped by the full suite source commit and implementation:

```text
conformance/reports/
└── <suite-revision>/
    └── example-fleet-manager/
        ├── README.md
        ├── v1.2.0-default-fleet-system-1.yaml
        └── v1.2.0-default-other-inventory-1.yaml
```

The layout is `<suite-revision>/<organization-project>/<run-id>.yaml`. The
organization and project match the report's implementation metadata. Use a
distinct run ID for each inventory, operating mode, or repeated execution.
Preserve earlier reports when adding results for a new run or version.

## Rules

Each implementation directory contains a `README.md` describing the
implementation, listing its reports, and explaining how to reproduce them.
Use the following structure:

```markdown
# Example Fleet Manager

[Fleet Manager](https://github.com/example/fleet-manager) publishes ClusterProfiles
for registered member clusters.

## Table of contents

| Implementation version | Mode | Inventory | Report |
| --- | --- | --- | --- |
| [v1.2.0](https://github.com/example/fleet-manager/releases/tag/v1.2.0) | default | fleet-system | [YAML](v1.2.0-default-fleet-system-1.yaml) |

## Reproduce

Include the environment preparation and test commands for the listed reports,
or link to instructions pinned to their implementation and suite revisions.
```

### Table of Contents

Include one row per submitted report. The implementation version, mode, and
inventory must match the YAML metadata, and the report link must resolve to its
file in this directory. Link the implementation version to its source snapshot
or release so readers can identify the tested implementation.

### Reproduce

The reproduction instructions must identify the Kubernetes version, deployed
implementation version or image, suite revision, registration and inventory
selection setup, readiness criteria, supported features, and execution command.
Explain any operating mode other than `default`. If steps differ between
versions or modes, identify which reports each set of instructions reproduces.
Credentials and machine-specific kubeconfigs are not submission artifacts.

## Reports

Submit the YAML exactly as emitted by the suite. HTML is available for inspecting
the same results locally; the YAML is the submission artifact.

### Generate a Submission Report

Use the implementation's own registration and inventory selection mechanisms to
prepare the test environment. Record the source commit of the implementation
actually deployed and the source commit of the suite actually executed. The
implementation commit is not necessarily the commit of a downstream test runner.

Check `git status --short` in both source checkouts before running: submission
sources must be clean. Source revisions and dirty flags are supplied by the
caller; the observer does not discover the deployed image's source or inspect
source checkouts. `--dirty` and `--suite-dirty` can describe development builds,
but either flag prevents submission acceptance.

The example below starts at this repository's root. It assumes that the deployed
`example/fleet-manager` version `v1.2.0` corresponds to the source checkout at
`IMPLEMENTATION_DIR`. Replace the example paths, connection settings, identity,
and claimed features with those of the implementation under test. Set the
reproduction URL to instructions or an artifact pinned to that implementation
revision.

```sh
SUITE_REVISION=$(git rev-parse HEAD)
IMPLEMENTATION_DIR=/absolute/path/fleet-manager
IMPLEMENTATION_REVISION=$(git -C "$IMPLEMENTATION_DIR" rev-parse HEAD)
IMPLEMENTATION_VERSION=v1.2.0
REPORT_DIR=$(mktemp -d)

cd conformance
go test -count=1 -run '^TestConformance$' -v . -args \
    --kubeconfig /absolute/path/hub.kubeconfig \
    --context inventory-hub \
    --namespace fleet-system \
    --cluster-manager fleet-manager \
    --expected-clusters 2 \
    --supported-features ClusterManagerLabel,InventoryMemberID,InventoryMemberUniqueness \
    --run-mode submission \
    --organization example \
    --project fleet-manager \
    --version "$IMPLEMENTATION_VERSION" \
    --revision "$IMPLEMENTATION_REVISION" \
    --mode default \
    --url https://github.com/example/fleet-manager \
    --contact @example/fleet-maintainers \
    --reproduction "https://github.com/example/fleet-manager/tree/$IMPLEMENTATION_REVISION/test/conformance" \
    --suite-version "$SUITE_REVISION" \
    --suite-revision "$SUITE_REVISION" \
    --report-format html,yaml \
    --report-dir "$REPORT_DIR"
```

Submission mode prohibits check filters and validates the generated report. The
two source revisions must be full commit hashes; versions identify a release or
source snapshot. For an imported suite, use its actual module version and source
commit. With a local workspace replacement, identify the replacement source
explicitly rather than reporting only the version declared in the importer's
`go.mod`.

### Validate the Report

From `conformance/`, validate the original YAML with the suite used for the run:

```sh
go run ./cmd/report validate "$REPORT_DIR/report.yaml"
```

The validator checks the schema and catalog, verifies each check's requirement
metadata, and recomputes counts and aggregate results. It accepts the catalog
compiled into that suite; use the matching suite for a report from another
catalog. Do not modify the report to make it pass validation.

### Reports rules

| Acceptance condition | Reason |
| --- | --- |
| Original YAML emitted by the suite | Preserves the observed results |
| Submission mode and a recognized schema and catalog | Excludes development runs and filtered results |
| Implementation and suite versions, full source commits, and clean source state | Identifies fixed test inputs |
| Organization, project, operating mode, URL, contacts, and reproduction reference | Identifies the implementation and how to reproduce the run |
| Specification and API revisions match the catalog | Prevents comparing different contracts as equivalent |
| Every catalog check appears once with unchanged requirement metadata | Prevents checks from being omitted or redefined |
| Outcomes, counts, and aggregate results agree | Prevents unsuccessful checks from being presented as successful |
| Observed Kubernetes version and inventory object references | Identifies the observed environment and snapshot |
| Result is `success` or `partial` | Excludes failed and undetermined runs |

Only `success` denotes conformance for Required checks and all claimed features.
`partial` records incomplete verification and remains visibly separate.
Unclaimed recommendations and capabilities remain visible in the report.

Validation checks the result artifact. It does not establish that declared
revisions match the deployed implementation, confirm that a reproduction URL is
immutable or usable, or execute that environment. Those checks belong to review
of the source references and reproduction instructions.

## Submit Results

After validation, copy the generated YAML into your implementation's directory.
Choose an unused run ID so earlier results are preserved. Continue from
`conformance/`, using the variables from the submission run:

```sh
SUBMISSION_DIR="reports/$SUITE_REVISION/example-fleet-manager"
mkdir -p "$SUBMISSION_DIR"
cp "$REPORT_DIR/report.yaml" \
    "$SUBMISSION_DIR/$IMPLEMENTATION_VERSION-default-fleet-system-1.yaml"
```

Add or update the implementation README according to [Rules](#rules).

Open a pull request against
[kubernetes-sigs/cluster-inventory-api](https://github.com/kubernetes-sigs/cluster-inventory-api)
containing the original YAML and the implementation README. A title such as
`Add Fleet Manager v1.2.0 conformance results` identifies the submission. Summarize
the tested version, suite revision, mode, inventory, and result in the pull
request description.

Before submitting, check that:

- The directory's suite revision and implementation name match the YAML metadata.
- The README table matches the submitted version, mode, inventory, and file name.
- The reproduction reference identifies fixed sources and the instructions can
  recreate the implementation-managed inventory and run the selected suite.
- The YAML is unchanged from the generated output and passes validation.

## Generate a Conformance Matrix

From `conformance/`, generate Markdown for the submitted reports of a suite
revision:

```sh
go run ./cmd/report matrix --reports-dir "reports/$SUITE_REVISION"
```

The command validates every YAML report under the selected directory before
writing output. A report with an unknown catalog or an invalid result aborts
generation. Use the suite matching the reports' catalog.

Rows retain implementation versions, modes, inventories, and run dates, grouped
by exact specification, suite, and API revisions. Partial results remain partial;
unclaimed features remain unclaimed. Links are relative to `--reports-dir`, so a
saved matrix belongs at the root of that report directory. Generating a matrix
does not upload results or publish a website.
