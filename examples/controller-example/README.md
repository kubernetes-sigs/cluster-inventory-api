# Controller Example

This example automatically sets up the following, stores the spoke cluster token in a Secret using the `secretreader` plugin, and lists spoke Pods from the `ClusterProfile`.

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

## Note: ClusterProfile extensions

- Required: set `status.accessProviders[].cluster.extensions[].name` to `client.authentication.k8s.io/exec`.
- The library reads only the `extension` field of that entry (arbitrary JSON). Other `extensions` entries are ignored.
- That `extension` is passed through to `ExecCredential.Spec.Cluster.Config`. The `secretreader` plugin uses `clusterName` in that object.

Example (to be merged into `ClusterProfile.status`):

```yaml
status:
  accessProviders:
  - name: secretreader
    cluster:
      server: https://<spoke-server>
      certificate-authority-data: <BASE64_CA>
      extensions:
      - name: client.authentication.k8s.io/exec
        extension:
          clusterName: spoke-1
```

Note: `client.authentication.k8s.io/exec` is a reserved key in the Kubernetes client authentication API. See the official documentation ("client.authentication.k8s.io").
