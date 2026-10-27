# API Versions

This document lists the API versions that are served, their planned removal, and
the changes in each release. The general compatibility and deprecation rules are
in the [compatibility policy](compatibility.md).

## Served API Versions

The `v0.2.0` CRDs serve these versions of `ClusterProfile` and `PlacementDecision`:

| API version | Status | Served | Storage version |
| --- | --- | --- | --- |
| `v1alpha1` | Deprecated | Yes | Yes |
| `v1alpha2` | Current | Yes | No |

Installing the `v0.2.0` CRDs does not migrate existing objects away from
`v1alpha1`.

Clients using either API version read and write the same stored objects without
a conversion webhook. For example, a cluster manager can keep writing a
`ClusterProfile` through `v1alpha1` while a consumer reads it through `v1alpha2`.

## Deprecation and Removal Notes

We plan to stop serving `v1alpha1` (`served: false`) in the release that introduces
the next API version after `v1alpha2`, such as `v1alpha3` or `v1beta1`, in
accordance with the [deprecation policy](compatibility.md#deprecation-policy).
The release version and date have not been set.

A storage version change and removal of the `v1alpha1` entry from `spec.versions`
have not been scheduled. Removing that entry requires
[migration of objects stored as `v1alpha1`](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#version-removal).

## v0.2.0

### Upgrade Notes

This release adds `multicluster.x-k8s.io/v1alpha2` for `ClusterProfile` and
`PlacementDecision`.

- `ClusterProfile.status.credentialProviders` is removed from `v1alpha2` and
  remains present and deprecated in `v1alpha1`. The replacement,
  `status.accessProviders`, has been available since `v0.1.0` and can be populated
  through either API version. There is no automatic conversion between these
  fields. Consumers that use access providers need the cluster manager to
  populate `status.accessProviders` before they switch to `v1alpha2`.
- `ClusterProfile` `v1alpha2` validates manager names and connection settings more
  strictly. `spec.clusterManager.name` must be a valid label value. Each
  `status.accessProviders` entry requires a `cluster` with an absolute `server`
  URL with a host; `proxy-url`, when set, must be a valid URL with scheme `http`,
  `https`, or `socks5` and a host.
- `PlacementDecision` `v1alpha2` requires `decisions[].clusterProfileRef.name` to
  be a valid DNS subdomain and `namespace`, when set, to be a valid DNS label.
- `pkg/access.BuildConfigFromCP` now takes a `*v1alpha2.ClusterProfile` and reads
  only `status.accessProviders`.

### Differences Between v1alpha1 and v1alpha2

These examples move an object from `v1alpha1` to `v1alpha2`. Comments mark where
`v1alpha2` differs.

```diff
-apiVersion: multicluster.x-k8s.io/v1alpha1
+apiVersion: multicluster.x-k8s.io/v1alpha2
 kind: ClusterProfile
 metadata:
   name: member-1
   namespace: fleet
   labels:
     x-k8s.io/cluster-manager: example-manager
 spec:
   displayName: member-1
   clusterManager:
     # v1alpha2: must be a valid label value.
     name: example-manager
 status:
   conditions:
   - type: ControlPlaneHealthy
     status: "True"
     reason: Healthy
     message: The control plane is healthy.
     lastTransitionTime: "2026-01-01T00:00:00Z"
   - type: Joined
     status: "True"
     reason: Joined
     message: The cluster has joined the fleet.
     lastTransitionTime: "2026-01-01T00:00:00Z"
   version:
     kubernetes: v1.37.0
   properties:
   - name: example.com/region
     value: us-east-1
     lastObservedTime: "2026-01-01T00:00:00Z"
-  # Deprecated. Exists only in v1alpha1.
-  credentialProviders:
-  - name: secretreader
-    cluster:
-      server: https://member-1.example.com
-      proxy-url: http://proxy.example.com:3128
   # Replacement for credentialProviders, with the same entry format.
   # Available in both v1alpha1 and v1alpha2. There is no conversion webhook,
   # so a cluster manager must populate this field itself.
   accessProviders:
   - name: secretreader
     cluster:
       # v1alpha2: must be an absolute URL with a host.
       server: https://member-1.example.com
       # v1alpha2: when set, must be an http, https, or socks5 URL with a host.
       proxy-url: http://proxy.example.com:3128
```

```diff
-apiVersion: multicluster.x-k8s.io/v1alpha1
+apiVersion: multicluster.x-k8s.io/v1alpha2
 kind: PlacementDecision
 metadata:
   name: placement-1
   namespace: fleet
 decisions:
 - clusterProfileRef:
     # v1alpha2: must be a valid DNS subdomain.
     name: member-1
     # v1alpha2: when set, must be a valid DNS label.
     namespace: fleet
```
