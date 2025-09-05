# Controller Example

This example automatically sets up the following, stores the spoke cluster token in a Secret using the `secretreader` plugin, and lists spoke Pods from the `ClusterProfile`.

- Create a hub cluster and a spoke cluster with kind
- On the spoke, create a ServiceAccount and ClusterRole/Binding that can list Pods and issue a token
- On the hub, create a Secret with the token in `data.token`
- On the hub, create a `ClusterProfile` with spoke information (set `secretreader` in `status.credentialProviders`)

## Prerequisites

- `kind`, `kubectl`, and `go` are available
- Working directory is the repository root

## 1. Run the setup script

Hub and spoke clusters will be created.

```bash
bash ./examples/controller-example/setup-kind-demo.sh
```

## 2. Build the Secret Reader plugin

```bash
go build -o ./bin/secretreader-plugin ./cmd/secretreader-plugin
```

## 3. Build the controller

```bash
go build -o ./examples/controller-example/controller-example.bin ./examples/controller-example
```

## 4. Run

```bash
KUBECONFIG=./examples/controller-example/hub.kubeconfig ./examples/controller-example/controller-example.bin \
  -clusterprofile-provider-file ./examples/controller-example/cp-creds.json \
  -namespace default \
  -clusterprofile spoke-1
```
