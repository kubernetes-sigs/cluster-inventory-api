# Controller Example - Kubeconfig Secret Reader plugin

This example uses the `kubeconfig-secretreader` plugin.

For plugin details (required RBAC, Secret format, `ExecCredential` output, and `ClusterProfile.status.accessProviders[].cluster.extensions`), see:

- [`plugins/kubeconfig-secretreader/cmd/plugin/README.md`](../../../../plugins/kubeconfig-secretreader/cmd/plugin/README.md)

It automatically sets up the following, stores the spoke cluster kubeconfig in a Secret using the `kubeconfig-secretreader` plugin, and lists spoke Pods from the `ClusterProfile`.

- Create a hub cluster and a spoke cluster with kind
- Extract kubeconfig from spoke cluster (server, CA, client cert/key)
- On the hub, create a Secret with kubeconfig YAML
- On the hub, create a `ClusterProfile` with spoke information (set `kubeconfig-secretreader` in `status.accessProviders`)

## Prerequisites

- `kind`, `kubectl`, and `go` are available
- Working directory is the repository root

## 1. Run the setup script

Hub and spoke clusters will be created.

```bash
bash ./examples/controller-example/plugins/kubeconfig-secretreader/setup-kind-demo.sh
```

## 2. Build the Kubeconfig Secret Reader plugin

```bash
make build-kubeconfig-secretreader-plugin
```

## 3. Build the controller example

```bash
make build-controller-example
```

## 4. Run

```bash
KUBECONFIG=./examples/controller-example/hub.kubeconfig ./examples/controller-example/controller-example.bin \
  -clusterprofile-provider-file ./examples/controller-example/plugins/kubeconfig-secretreader/provider-config.json \
  -namespace default \
  -clusterprofile spoke-1
```

## Note: ClusterProfile extensions for Kubeconfig Secret Reader plugin

This example relies on `ClusterProfile.status.accessProviders[].cluster.extensions` to pass Secret lookup information to the plugin.
The authoritative description (including an example and namespace inference behavior) lives in:

- [`plugins/kubeconfig-secretreader/cmd/plugin/README.md`](../../../../plugins/kubeconfig-secretreader/cmd/plugin/README.md)
