# Compatibility Policy

This policy defines compatibility and deprecation expectations for CRD APIs,
documented public Go packages, and documented plugin image contracts in this
repository. It is based on Kubernetes API compatibility and deprecation guidance
where that guidance applies to CRD APIs.

See [API Versions](api-versions.md) for the API versions currently served and
their removal plans.

## Scope

This policy applies to:

- CRD APIs under `apis/` and generated CRD manifests under `config/crd/`.
- Documented public Go packages intended for consumers, including `pkg/access`.
- Documented plugin image contracts for released images, including command-line
  behavior, input configuration, and `ExecCredential` output.

This policy does not apply to:

- Unreleased changes on the default branch.
- Test code, hack scripts, examples, and local development tooling unless a
  release note explicitly documents them as supported interfaces.
- Internal implementation details that are not part of a documented API or
  package contract.

## Versions

A repository release and a CRD API version are versioned separately:

| Version | Example | Applies to |
| --- | --- | --- |
| Repository release | `v0.2.0` | CRDs, Go packages, and plugin images published in that release |
| CRD API version | `multicluster.x-k8s.io/v1alpha2` | The API schema and endpoint used by ClusterProfile and PlacementDecision clients |

A repository release can serve more than one CRD API version.

## API Compatibility

A change is compatible when existing API consumers and stored objects that do not
use the new behavior continue to work with the same semantics after the change.

Compatible changes include:

- Adding an optional field with a backward-compatible default.
- Adding a new status field, condition type, or condition reason when API
  consumers are expected to tolerate unknown status values.
- Adding a new served CRD version.
- Loosening validation when existing semantics remain unchanged.
- Adding new plugin options that are optional and do not change existing option
  behavior.

Incompatible changes include:

- Removing or renaming a CRD field, JSON field name, resource, or served CRD
  version.
- Adding a new required field to an existing API version.
- Changing the default value or semantic meaning of an existing field or value.
- Making a mutable field immutable, or otherwise changing update semantics.
- Tightening validation in a way that rejects previously accepted requests or
  stored objects.
- Removing a public Go type, function, method, or field.
- Changing documented plugin input or output semantics.

When a change is ambiguous, treat it as incompatible unless the PR explains why
existing users, stored objects, rollback, and version skew continue to work.

## Deprecation Policy

Incompatible changes should go through deprecation before removal.

Deprecating an API or contract requires:

- Marking the Go API comment with `Deprecated:` when the deprecated interface is
  represented in Go.
- Marking CRD fields with the appropriate kubebuilder deprecation markers when
  applicable.
- Documenting the replacement and migration path in user-facing docs or release
  notes.
- Emitting a runtime warning when practical and when the deprecated path is used.
- Recording the intended removal release or removal condition when known.

Deprecated interfaces may be removed only when:

- A replacement has been available in at least one released version.
- The migration path has been documented.
- The deprecation period for the relevant stability level has elapsed.
- The removal is called out in release notes.

For alpha CRD APIs, incompatible changes are allowed, but replacements for
previously released interfaces should still include migration guidance. When
practical, keep a deprecated alpha field or behavior for at least one minor
release or three months after the replacement is released, whichever is longer.
Shorter periods require OWNER approval and should be reserved for security
fixes, incorrect behavior, or interfaces that were never included in a release.

When removing a field from a released alpha CRD API, prefer removing it from a
new API version instead of directly changing the existing served version. The
old version should remain served with the deprecated field until the documented
deprecation period or approved exception has elapsed.

For beta or stable CRD APIs, follow the Kubernetes deprecation policy. In
particular, beta API versions require a longer deprecation window before
removal, and stable APIs must not be removed without a major-version policy.

When introducing a new served CRD API version, continue serving the immediately
preceding version. Older versions may stop being served in the same release if
they meet the deprecation requirements in this section.

For documented public Go packages and plugin image contracts while this
repository remains in the v0.x release series, incompatible changes may be made
in minor releases when they are documented. Patch releases must not contain
incompatible changes except for security or critical correctness fixes.

## Version Skew

Released Go packages and plugin images are expected to work with the CRDs from
the same release.

When a release introduces a replacement API field or behavior:

- Code in this repository that consumes the API should tolerate both the
  deprecated and replacement forms during the documented deprecation period.
- The replacement should be preferred when both forms are present.
- Removal should happen in a later minor release unless an exception applies.

When a new CRD version is introduced:

- Do not make a new storage version in the same release unless rollback and
  stored-object compatibility are addressed.
- Do not require a conversion webhook for simple alpha version bumps unless the
  API needs semantic conversion that Kubernetes cannot handle with its normal
  CRD version handling.
- Document supported upgrade paths in release notes or a dedicated versions
  document.

Supported Kubernetes versions are documented separately by the project security
policy and release notes.

## Exceptions

The following changes may bypass the normal deprecation period with OWNER
approval:

- Security fixes.
- Fixes for behavior that is clearly incorrect or dangerous.
- Removal of alpha functionality that has not been included in a release.
- Removal of functionality whose continued support would block a required
  Kubernetes compatibility fix.

Even when an exception applies, PRs should describe user impact and migration
guidance.

## References

- [Kubernetes API change guidance](https://github.com/kubernetes/community/blob/main/contributors/devel/sig-architecture/api_changes.md)
- [Kubernetes deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/)
