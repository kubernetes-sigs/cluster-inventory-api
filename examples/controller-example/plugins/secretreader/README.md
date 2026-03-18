# Controller Example - Secret Reader plugin

This example uses the `secretreader` plugin.

For plugin details (required RBAC and `ClusterProfile.status.accessProviders[].cluster.extensions`), see:

- [`plugins/secretreader/cmd/plugin/README.md`](../../../../plugins/secretreader/cmd/plugin/README.md)

It automatically sets up the following, stores the spoke cluster token in a Secret using the `secretreader` plugin, and lists spoke Pods from the `ClusterProfile`.

- Create a hub cluster and a spoke cluster with kind
- On the spoke, create a ServiceAccount and ClusterRole/Binding that can list Pods and issue a token
- On the hub, create a Secret with the token in `data.token`
- On the hub, create a `ClusterProfile` with spoke information (set `secretreader` in `status.accessProviders`)

## Prerequisites

- `kind`, `kubectl`, and `go` are available
- Working directory is the repository root

## 1. Run the setup script

Hub and spoke clusters will be created.

```bash
bash ./examples/controller-example/plugins/secretreader/setup-kind-demo.sh
```

## 2. Build the Secret Reader plugin

```bash
make build-secretreader-plugin
```

## 3. Build the controller example

```bash
make build-controller-example
```

## 4. Run

```bash
KUBECONFIG=./examples/controller-example/hub.kubeconfig ./examples/controller-example/controller-example.bin \
  -clusterprofile-provider-file ./examples/controller-example/plugins/secretreader/provider-config.json \
  -namespace default \
  -clusterprofile spoke-1
```

## Note: ClusterProfile extensions for Secret Reader plugin

This example relies on `ClusterProfile.status.accessProviders[].cluster.extensions` to pass Secret lookup information to the plugin.
The authoritative description (including an example) lives in:

- [`plugins/secretreader/cmd/plugin/README.md`](../../../../plugins/secretreader/cmd/plugin/README.md)
