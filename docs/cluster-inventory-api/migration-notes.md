# Migration Notes

This page provides field-level mappings for projects migrating from proprietary cluster registry APIs to the Cluster Inventory API (`ClusterProfile`). These are not exhaustive — the goal is to help you find the equivalent ClusterProfile field for each concept you already use.

---

## Open Cluster Management — `ManagedCluster` → `ClusterProfile`

OCM's hub-side [ClusterProfile reconciler](https://github.com/open-cluster-management-io/ocm/tree/main/pkg/registration/hub/clusterprofile) (available since OCM v0.15.0) performs this mapping automatically. The table below shows how fields correspond.

| OCM `ManagedCluster` field | ClusterProfile field | Notes |
|---------------------------|----------------------|-------|
| `metadata.name` | `metadata.name` | 1:1 |
| — | `metadata.labels["x-k8s.io/cluster-manager"]` | Set to `"open-cluster-management"` by OCM |
| — | `spec.clusterManager.name` | Set to `"open-cluster-management"` |
| `spec.hubAcceptsClient` | — | OCM-specific; no equivalent |
| `status.conditions["ManagedClusterConditionAvailable"]` | `status.conditions["ControlPlaneHealthy"]` | Status/reason copied |
| `status.version.kubernetes` | `status.version.kubernetes` | Direct copy |
| `status.clusterClaims[name="id.k8s.io"]` | `status.properties["clusterset.k8s.io/cluster-id"]` | Via ClusterProperty resource |
| `status.clusterClaims[name="platform.open-cluster-management.io"]` | `status.properties["platform.open-cluster-management.io"]` | Custom property |
| `status.clusterClaims[name="region.open-cluster-management.io"]` | `status.properties["topology.kubernetes.io/region"]` | Mapped to standard name |
| cluster-proxy addon endpoint | `status.accessProviders[name="open-cluster-management"]` | Server, CA, and exec extension populated by OCM |

**Namespace strategy**: OCM publishes `ClusterProfile` objects in namespaces named after the `ManagedClusterSet`, each labeled `multicluster.x-k8s.io/clusterset=<set-name>`.

---

## Karmada — `Cluster` → `ClusterProfile`

No automated sync exists yet. If you are building one, the mapping below guides the implementation.

| Karmada `Cluster` field | ClusterProfile field | Notes |
|------------------------|----------------------|-------|
| `metadata.name` | `metadata.name` | 1:1 |
| — | `metadata.labels["x-k8s.io/cluster-manager"]` | Set to your manager name (e.g. `"karmada"`) |
| — | `spec.clusterManager.name` | Same value |
| `spec.apiEndpoint` | `status.accessProviders[].cluster.server` | API server URL |
| `spec.secretRef` | — | Karmada-specific secret reference; credentials exposed via exec plugin instead |
| `spec.syncMode` | — | No ClusterProfile equivalent (Karmada-specific) |
| `status.conditions["Ready"]` | `status.conditions["ControlPlaneHealthy"]` | Map `True`/`False`/`Unknown` directly |
| `status.kubernetesVersion` | `status.version.kubernetes` | Direct copy |
| `status.apiEnablements` | — | No standard equivalent; use a custom property if needed |
| `status.nodeSummary.totalNum` | `status.properties["my-manager.io/node-count"]` | Custom property |

**Credential approach**: Karmada stores credentials in a `Secret` referenced by `spec.secretRef`. When migrating, use the `kubeconfig-secretreader` built-in plugin to expose the kubeconfig from that secret via `status.accessProviders`.

---

## GKE Fleet — `MemberCluster` / Fleet Membership → `ClusterProfile`

GKE Fleet's [ClusterProfile sync](https://cloud.google.com/kubernetes-engine/fleet-management/docs/generate-inventory-for-integrations) (Preview) performs this mapping automatically. Profiles are published in the `fleet-cluster-inventory` namespace.

| GKE Fleet concept | ClusterProfile field | Notes |
|------------------|----------------------|-------|
| Fleet membership name | `metadata.name` | Typically `<project-id>_<location>_<cluster-name>` |
| — | `metadata.labels["x-k8s.io/cluster-manager"]` | Set to `"gke-fleet"` |
| — | `spec.clusterManager.name` | Set to `"gke-fleet"` |
| Membership endpoint (API server URL) | `status.accessProviders[].cluster.server` | Populated by Fleet sync |
| Membership CA certificate | `status.accessProviders[].cluster.certificateAuthorityData` | Populated by Fleet sync |
| `MEMBERSHIP_STATE_ACTIVE` | `status.conditions["ControlPlaneHealthy"] = True` | Mapped by Fleet sync |
| Location/region metadata | `status.properties["topology.kubernetes.io/region"]` | Populated by Fleet sync |
| Fleet membership labels | `metadata.labels` (pass-through) | Fleet sync copies membership labels |

**Default namespace**: `fleet-cluster-inventory` (configurable).

**Access provider name**: `"gke-fleet"`. Consumers targeting GKE Fleet profiles should configure their provider config file to use the `"gke-fleet"` name.

---

## Summary: Common Field Mapping Patterns

| Concept | ClusterProfile canonical location |
|---------|----------------------------------|
| Cluster identity / manager | `spec.clusterManager.name` + `x-k8s.io/cluster-manager` label |
| Cluster health | `status.conditions["ControlPlaneHealthy"]` |
| Kubernetes version | `status.version.kubernetes` |
| Stable cluster ID | `status.properties["clusterset.k8s.io/cluster-id"]` |
| Region | `status.properties["topology.kubernetes.io/region"]` |
| Zone | `status.properties["topology.kubernetes.io/zone"]` |
| API server URL | `status.accessProviders[].cluster.server` |
| CA certificate | `status.accessProviders[].cluster.certificateAuthorityData` |
| Exec credential config | `status.accessProviders[].cluster.extensions["client.authentication.k8s.io/exec"]` |
| ClusterSet membership | Namespace labeled `multicluster.x-k8s.io/clusterset=<name>` |
