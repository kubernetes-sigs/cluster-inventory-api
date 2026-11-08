# Cluster Inventory API Implementations

This document tracks downstream implementations and integrations of the Cluster Inventory API (ClusterProfile API) and provides status and resource references for them.

For background on the API itself, see the [ClusterProfile API Overview](../../concepts/cluster-profile-api.md), [KEP-4322 (Cluster Inventory API)](https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/4322-cluster-inventory), and [KEP-5339 (ClusterProfile credentials plugin)](https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/5339-clusterprofile-plugin-credentials), which defines the pluggable exec-based mechanism consumers use to obtain credentials for member clusters.

Implementors and integrators of the Cluster Inventory API are encouraged to update this document with status information about their implementations, the versions they cover, and documentation to help users get started.

The Cluster Inventory API has two kinds of implementations:

- **Cluster Managers** publish `ClusterProfile` objects for the member clusters they register.
- **ClusterProfile API Consumers** read `ClusterProfile` objects and use them to schedule, deploy, or operate workloads across clusters. The consumers listed below typically resolve per-cluster credentials via the [KEP-5339](https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/5339-clusterprofile-plugin-credentials) exec credential plugin mechanism (often through the shared `sigs.k8s.io/cluster-inventory-api` library); some consumers additionally provide built-in credential shortcuts outside that mechanism for specific environments.

## Getting Started Guides

- **[Guide for Cluster Manager Implementers](./guide-cluster-manager-implementers.md)** — step-by-step guide for building a cluster manager that publishes `ClusterProfile` objects.
- **[Guide for ClusterProfile Consumers](./guide-cluster-profile-consumers.md)** — step-by-step guide for building a controller, scheduler, or tool that reads `ClusterProfile` objects and connects to member clusters.
- **[Migration Notes](./migration-notes.md)** — field-level mappings for projects migrating from OCM `ManagedCluster`, Karmada `Cluster`, or GKE Fleet memberships.

## Implementation Status

### Cluster Managers

- [Open Cluster Management][ocm]: Available (shipped since OCM v0.15.0, enhanced in v1.2.0; PlacementDecision API support in progress)
- [GKE Fleet (ClusterProfile sync)][gke-fleet-sync]: Preview (Google Cloud, announced May 2025)

### ClusterProfile API Consumers

- [Headlamp][headlamp]: Alpha (since Headlamp v0.43.0)
- [Knative Operator][knative-operator]: Alpha (since Knative Operator v1.22)
- [Kueue (MultiKueue)][kueue]: Alpha (since Kueue v0.15.0, behind the `MultiKueueClusterProfile` feature gate)
- [multicluster-runtime][mcr]: Alpha (since v0.21.0-alpha.9)
- [Argo CD][argocd]: In review (implementation in [argoproj/argo-cd#24509][argocd-pr]; unreleased, target version not yet determined)

## Implementations

In this section you will find specific links to code, documentation, and other Cluster Inventory API references for specific implementations.

The consumer implementations target different layers:

- Use **Headlamp** when you want a Kubernetes web UI to discover and register clusters from `ClusterProfile` objects.
- Use the **Knative Operator** to roll out Knative Serving and Eventing to member clusters.
- Use **MultiKueue** when the workloads you dispatch across clusters are batch jobs.
- Use **multicluster-runtime** when you are building your own controller on top of controller-runtime and want `ClusterProfile`-driven fleet discovery.
- Use **Argo CD** to deliver applications across clusters via GitOps (integration under review upstream, not yet merged).

### Open Cluster Management

[Open Cluster Management (OCM)][ocm] is a CNCF sandbox project that provides multicluster management APIs and controllers for Kubernetes. OCM acts as a ClusterProfile provider: its [hub-side ClusterProfile reconciler][ocm-clusterprofile] synchronizes each registered `ManagedCluster` into a `ClusterProfile` object. OCM also supplies the `open-cluster-management` access provider through its [cluster-proxy][ocm-cluster-proxy] and [managed-serviceaccount][ocm-managed-sa] addons, allowing consumers to obtain credentials via the client-go exec credential plugin mechanism.

Initial ClusterProfile support was introduced in [OCM v0.15.0 (October 2024)][ocm-v0-15]. The implementation was updated in [OCM v1.2.0 (February 2026)][ocm-v1-2], where the reconciler was refactored to sync from `ManagedClusterSet` and the `ClusterProfile` spec and status fields were revised.

OCM is also tracking support for the SIG Multicluster [PlacementDecision API][kep-5313] in [open-cluster-management-io/ocm#1373][ocm-placementdecision-issue]. The proposal would allow OCM to produce standardized `PlacementDecision` objects from its placement results, giving third-party consumers such as Argo CD a common interface instead of provider-specific placement integrations. If completed, this could make OCM one of the first cluster managers to produce standardized SIG Multicluster placement decisions.

[ocm]: https://open-cluster-management.io/
[ocm-clusterprofile]: https://github.com/open-cluster-management-io/ocm/tree/main/pkg/registration/hub/clusterprofile
[ocm-cluster-proxy]: https://github.com/open-cluster-management-io/cluster-proxy/blob/main/pkg/proxyserver/controllers/clusterprofile_controller.go
[ocm-managed-sa]: https://github.com/open-cluster-management-io/managed-serviceaccount/blob/main/pkg/addon/manager/controller/clusterprofile_cred_syncer.go
[ocm-placementdecision-issue]: https://github.com/open-cluster-management-io/ocm/issues/1373
[ocm-v0-15]: https://open-cluster-management.io/docs/release/#0150-24-oct-2024
[ocm-v1-2]: https://open-cluster-management.io/docs/release/#120-2-february-2026
[kep-5313]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-multicluster/5313-placement-decision-api/README.md

### GKE Fleet (ClusterProfile sync)

[GKE Fleet][gke-fleet] is Google Cloud's fleet management layer for Google Kubernetes Engine. Its [ClusterProfile sync][gke-fleet-sync] feature, available in Preview under the Pre-GA Offerings Terms, acts as a ClusterProfile provider: Fleet is the source of truth, and fleet membership changes (additions, updates, and deletions) are one-way synchronized to `ClusterProfile` objects on a designated hub cluster. Generated profiles are published in the `fleet-cluster-inventory` namespace by default and carry the label `x-k8s.io/cluster-manager=gke-fleet`.

The feature was announced in the [May 8, 2025 GKE release notes][gke-fleet-sync-release]. To enable it, an operator designates a GKE cluster as the hub by setting the `fleet-clusterinventory-management-cluster=true` label. The documented procedure currently targets GKE clusters; behavior for non-GKE fleet members (such as attached clusters) is not covered by the Preview documentation. Google documents the [Argo CD ClusterProfile Syncer][gke-argocd-syncer] and [Multi-cluster Orchestrator][gke-mco] as example consumers; the syncer is a GKE-focused, Google-maintained integration, distinct from the upstream [Argo CD](#argo-cd) proposal described below.

[gke-fleet]: https://cloud.google.com/kubernetes-engine/fleet-management/docs
[gke-fleet-sync]: https://cloud.google.com/kubernetes-engine/fleet-management/docs/generate-inventory-for-integrations
[gke-fleet-sync-release]: https://cloud.google.com/kubernetes-engine/docs/release-notes#May_08_2025
[gke-argocd-syncer]: https://github.com/GoogleCloudPlatform/gke-fleet-management/tree/main/argocd-clusterprofile-syncer
[gke-mco]: https://cloud.google.com/blog/products/containers-kubernetes/multi-cluster-orchestrator-for-cross-region-kubernetes-workloads/

### Headlamp

[Headlamp][headlamp] is an extensible Kubernetes web UI. Its Cluster Inventory integration watches `ClusterProfile` resources, builds Kubernetes clients from configured access providers, and registers discovered clusters in Headlamp so users can browse and manage them from the UI without manually adding each cluster to a kubeconfig.

The feature is alpha/experimental, disabled by default, and can be enabled with the `--enable-cluster-inventory` flag. It was added in [kubernetes-sigs/headlamp#4577][headlamp-pr] and is available since Headlamp v0.43.0.

[headlamp]: https://headlamp.dev/
[headlamp-pr]: https://github.com/kubernetes-sigs/headlamp/pull/4577

### Knative Operator

[Knative][knative] provides serverless workloads on Kubernetes. The [Knative Operator][knative-operator] consumes the Cluster Inventory API to deploy Knative Serving and Eventing across a fleet of remote clusters: when `spec.clusterProfileRef` is set on a `KnativeServing` or `KnativeEventing` resource, the operator resolves the referenced `ClusterProfile`, uses a configured access provider to build a REST config, and applies manifests to the target member cluster.

See the [multicluster deployment guide][knative-operator-mc] for details. The feature was added via [knative/operator#2267][knative-operator-pr] and is available since Knative Operator v1.22.

[knative]: https://knative.dev/
[knative-operator]: https://github.com/knative/operator
[knative-operator-mc]: https://github.com/knative/operator/blob/main/docs/multicluster.md
[knative-operator-pr]: https://github.com/knative/operator/pull/2267

### Kueue (MultiKueue)

[Kueue][kueue] is a cloud-native job queueing system for batch, HPC, AI/ML, and similar applications in Kubernetes. Its multicluster mode, [MultiKueue][kueue-multikueue], dispatches workloads from a manager cluster to worker clusters. Since Kueue v0.15.0, MultiKueue can consume the Cluster Inventory API as a source of worker-cluster connection details: a `MultiKueueCluster` may reference a `ClusterProfile`, and the manager uses the referenced object's `accessProviders` together with a client-go exec credential plugin to reach the worker cluster.

The integration is an alpha feature gated on `MultiKueueClusterProfile`. See the [MultiKueue setup guide][kueue-multikueue-cp] for configuration details.

[kueue]: https://kueue.sigs.k8s.io/
[kueue-multikueue]: https://github.com/kubernetes-sigs/kueue/tree/main/keps/693-multikueue
[kueue-multikueue-cp]: https://kueue.sigs.k8s.io/docs/tasks/manage/setup_multikueue/#configure-federated-credential-discovery-via-the-clusterprofile-api

### multicluster-runtime

[multicluster-runtime][mcr] is a SIG Multicluster library that extends [controller-runtime][controller-runtime] with a pluggable provider model for reconciling across a fleet of clusters. It ships a reference [Cluster Inventory API provider][mcr-cip-provider] that discovers member clusters from `ClusterProfile` objects and, via its [kubeconfigstrategy][mcr-cip-kcfg] package, resolves each `ClusterProfile` to a `*rest.Config` through a pluggable strategy (for example, by building the config from the cluster's `status.accessProviders` and invoking a configured client-go exec credential plugin).

The Cluster Inventory API provider was introduced in [v0.21.0-alpha.9 (August 2025)][mcr-v0-21-a9]. An [end-to-end example][mcr-cip-example] is available in the repository.

[mcr]: https://github.com/kubernetes-sigs/multicluster-runtime
[controller-runtime]: https://github.com/kubernetes-sigs/controller-runtime
[mcr-cip-provider]: https://github.com/kubernetes-sigs/multicluster-runtime/tree/main/providers/cluster-inventory-api
[mcr-cip-kcfg]: https://github.com/kubernetes-sigs/multicluster-runtime/tree/main/providers/cluster-inventory-api/kubeconfigstrategy
[mcr-cip-example]: https://github.com/kubernetes-sigs/multicluster-runtime/tree/main/examples/cluster-inventory-api
[mcr-v0-21-a9]: https://github.com/kubernetes-sigs/multicluster-runtime/releases/tag/v0.21.0-alpha.9

### Argo CD

[Argo CD][argocd] is a declarative, GitOps continuous delivery tool for Kubernetes. A ClusterProfile integration is under review in [argoproj/argo-cd#24509][argocd-pr] (tracking [argoproj/argo-cd#24282][argocd-issue]); it is not yet merged, and the release version that will first carry the feature is not yet determined.

The proposal introduces a dedicated controller that watches `ClusterProfile` resources and materializes a corresponding Argo CD cluster Secret for each profile, so that the existing Argo CD application-sync machinery can target `ClusterProfile`-registered member clusters. The design includes two credential modes: built-in cloud-provider shortcuts, and a [KEP-5339][kep-5339] exec credential plugin mode. The integration is planned to support both [KEP-4322][kep-4322] (as a `ClusterProfile` reader) and [KEP-5339][kep-5339] (via the exec credential plugin path). The configuration surface is still in flux; see the PR for current details. This upstream proposal is separate from the Google-maintained [Argo CD ClusterProfile Syncer][gke-argocd-syncer] described in the [GKE Fleet](#gke-fleet-clusterprofile-sync) section.

[argocd]: https://argo-cd.readthedocs.io/
[argocd-pr]: https://github.com/argoproj/argo-cd/pull/24509
[argocd-issue]: https://github.com/argoproj/argo-cd/issues/24282
[kep-4322]: https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/4322-cluster-inventory
[kep-5339]: https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/5339-clusterprofile-plugin-credentials
