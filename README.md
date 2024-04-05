# ClusterInventory API

The `ClusterInventory API` provides a reliable, consistent, and automated approach for any multi-cluster application (framework, toolset) to discover available clusters and take actions accordingly, in a way similar to service discovery works in a microservice architecture. Through the inventory, the application can query for a list of clusters to access, or watch for an ever-flowing stream of cluster lifecycle events which the application can act upon timely, such as auto-scaling, upgrades, failures, and connectivity issues.
This repo will hold design documents and implementation of the cluster inventory API as described in [KEP 4322](https://github.com/kubernetes/enhancements/blob/master/keps/sig-multicluster/4322-cluster-inventory/README.md) and [Discussion doc](https://docs.google.com/document/d/1sUWbe81BTclQ4Uax3flnCoKtEWngH-JA9MyCqljJCBM/).

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
