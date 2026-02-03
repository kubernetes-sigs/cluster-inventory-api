# Cluster Inventory API

The Cluster Inventory API is a part of [SIG Multicluster](https://github.com/kubernetes/community/tree/master/sig-multicluster),
and this repository serves as the foundation for developing a standardized,
robust framework for multi-cluster management in a cloud-native environment.

The Cluster Inventory API aims to provide a consistent and automated approach for applications,
frameworks, and toolsets to discover, interact with, and make placement decisions across multiple Kubernetes clusters.
The concept of Cluster Inventory is akin to service discovery in a microservices architecture.
It allows multi-cluster applications to dynamically discover available clusters and respond to various cluster lifecycle events.
Such events include auto-scaling, upgrades, failures, and connectivity issues.
This automated inventory management not only facilitates operational efficiency
but also supports the integration of diverse multi-cluster management solutions.
See the initial proposal in the [documentation](https://docs.google.com/document/d/1sUWbe81BTclQ4Uax3flnCoKtEWngH-JA9MyCqljJCBM/)
for more details.

## Cluster Profile API

Within the broader Cluster Inventory, the first major component we are introducing is the
[ClusterProfile API](https://github.com/kubernetes/enhancements/blob/master/keps/sig-multicluster/4322-cluster-inventory/README.md).
A Cluster Profile is essentially an individual member of the Cluster Inventory that details the properties and status of a cluster.
This API proposes a universal, standardized interface that defines how cluster information should be presented
and interacted with across different platforms and implementations.

### Motivation and Goals

The `ClusterProfile API` is designed to establish a shared interface for cluster inventory,
laying the groundwork for multi-cluster tooling by providing a foundational component.
Here are several key benefits and purposes of adopting the ClusterProfile API:

- **Standardization**: By defining a standard for status reporting and cluster properties,
the API facilitates a common understanding that can be shared across various tools and platforms.
- **Ease of Integration**: Consumers of the API, such as multi-cluster workload schedulers and GitOps tools (e.g., ArgoCD, Kueue),
can integrate without needing to navigate the specific implementation details of different cluster management projects.
- **Vendor Neutrality**: The API provides a vendor-neutral integration point,
allowing operations tools and external consumers to define and manage clusters across different cloud environments uniformly.

## PlacementDecision API

The [PlacementDecision API](https://github.com/kubernetes/enhancements/blob/master/keps/sig-multicluster/5313-placement-decision/README.md)
is a vendor-neutral API that standardizes the output of multicluster placement calculations.
A `PlacementDecision` object is data only: a namespaced list of chosen clusters whose referenced names
must map one-to-one to `ClusterProfile`s. Any scheduler can emit the object and any consumer can watch it.

### Motivation and Goals

Today every multicluster scheduler publishes its own API to convey where a workload should run,
forcing downstream tools such as GitOps engines, workload orchestrators, progressive rollout controllers,
or AI/ML pipelines to understand a scheduler-specific API.

The `PlacementDecision API` solves the integration explosion problem:

- **Reduces integration burden**: Consumers write ONE integration that works with ANY scheduler implementing this API.
- **Enables scheduler portability**: Switch schedulers without rewriting consumer code.
- **Enables consumer portability**: New consumers work with all schedulers by implementing one standard API.
- **Simplifies RBAC**: One resource schema to secure instead of different permissions for each scheduler's API.

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack](https://kubernetes.slack.com/messages/sig-multicluster)
- [Mailing List](https://groups.google.com/forum/#!forum/kubernetes-sig-multicluster)
- [SIG Multicluster](https://github.com/kubernetes/community/blob/master/sig-multicluster/README.md)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
