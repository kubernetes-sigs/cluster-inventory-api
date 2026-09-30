# API Versions and Migration Notes

This document records API version changes that require user action during
upgrades. Release notes should copy the relevant entry when a release includes
one of these changes.

## v0.2.0

### Upgrade Notes

- The ClusterProfile CRD now serves a new
  `multicluster.x-k8s.io/v1alpha2` API version.

  The `v1alpha2` ClusterProfile status removes the deprecated
  `status.credentialProviders` field. Use `status.accessProviders` instead.
  `status.accessProviders` has been available in a released version since
  `v0.1.0`.

- `v1alpha2` clients and `pkg/access` in `v0.2.0` do not read
  `status.credentialProviders`.

  If a cluster manager only populates `status.credentialProviders`, update it
  to also populate `status.accessProviders` before switching consumers to
  `v1alpha2` or `pkg/access` from `v0.2.0`.

- `v1alpha2` validates ClusterProfile data more strictly.

- The PlacementDecision CRD now serves a new
  `multicluster.x-k8s.io/v1alpha2` API version.

  The `v1alpha2` API validates PlacementDecision data more strictly.

### Upgrade Steps

#### 1. Verify ClusterProfile Access Providers

Before upgrading consumers, ensure cluster managers populate
`ClusterProfile.status.accessProviders`.

#### 2. Install v0.2.0 CRDs

Install the `v0.2.0` CRDs. The `v1alpha1` API version remains served and remains
the storage version in `v0.2.0`, so existing stored `v1alpha1` objects are not
migrated to a new storage version during this release.

#### 3. Update ClusterProfile apiVersion References

Update manifests and clients that create or read `ClusterProfile` objects to use
`apiVersion: multicluster.x-k8s.io/v1alpha2` when they no longer depend on
`status.credentialProviders`.

#### 4. Update pkg/access Go Consumers

Update Go consumers of `pkg/access` to pass `*v1alpha2.ClusterProfile` to
`BuildConfigFromCP`.

#### 5. Update PlacementDecision apiVersion References

Consumers can move `PlacementDecision` manifests and clients to
`apiVersion: multicluster.x-k8s.io/v1alpha2` when they are ready to stop using
the deprecated `v1alpha1` endpoint.

### Deprecation and Removal Notes

- `ClusterProfile.status.credentialProviders` remains present and deprecated in
  `v1alpha1` for compatibility.
- `ClusterProfile.status.credentialProviders` is not present in
  `ClusterProfile` `v1alpha2`.
- `ClusterProfile` and `PlacementDecision` `v1alpha1` remain served and remain
  the storage versions in `v0.2.0`.

### Release Note

```text
The ClusterProfile CRD now serves a new multicluster.x-k8s.io/v1alpha2 API
version. The v1alpha2 ClusterProfile status removes the deprecated
status.credentialProviders field; use status.accessProviders instead.
status.accessProviders has been available since v0.1.0. The v1alpha1 API
version remains served and remains the storage version in v0.2.0, with
status.credentialProviders retained there for compatibility. Consumers using
pkg/access in v0.2.0 must pass v1alpha2 ClusterProfile objects and should ensure
their cluster managers populate status.accessProviders before switching.
The v1alpha2 API also validates ClusterProfile data more strictly.

The PlacementDecision CRD also now serves multicluster.x-k8s.io/v1alpha2.
The v1alpha2 API validates PlacementDecision data more strictly. The v1alpha1
API version remains served and remains the storage version in v0.2.0.
```
