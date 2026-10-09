# Accepted conformance reports

This directory is the canonical destination for implementation conformance
reports. Each report describes one implementation version and operating mode,
observing one inventory under a fixed suite and specification.

The layout is `<suite-revision>/<organization-project>/<run-id>.yaml`.
For example, a manager's report belongs to
`0123456789abcdef0123456789abcdef01234567/example-fleet/20261009-default.yaml`.
Run IDs distinguish inventories, modes, and repeated executions. Historical
results remain separate; a new report does not retroactively change an older run.

| Acceptance condition | Reason |
| --- | --- |
| Original YAML emitted by the suite, without result edits | Results remain reproducible |
| Submission mode and known versioned schema/catalog | Development runs cannot claim full acceptance |
| Implementation and suite versions with full commits and clean source trees | Identifies fixed test inputs |
| Organization, project, operating mode, URL, maintainer contacts, and pinned reproduction reference | Identifies ownership and how the same execution can be reproduced |
| Specification and API revisions match the tested catalog | Prevents comparing different contracts as equivalent |
| Every catalog check appears exactly once, with unchanged requirement metadata | Required checks cannot disappear |
| Outcomes, counts, and aggregate results agree | Failed, skipped, or unknown checks cannot become successful |
| Observed Kubernetes version and inventory object references | Identifies the observed environment and snapshot |
| Result is success or partial | Failure and unknown are not accepted conformance results |

Only `success` denotes conformance for Required checks and all claimed features.
`partial` denotes an incomplete verification and stays visibly separate.
Unclaimed recommendations remain visible in the report. Acceptance checks verify
the result artifact; they do not independently attest the implementation's source
or execute its reproduction environment.

The validator accepts the catalog compiled into its suite. A report for another
catalog requires that catalog's suite, rather than relaxing validation to accept
unknown requirements. The matrix generator includes all accepted runs and groups
them by exact specification, suite, and API revisions.
