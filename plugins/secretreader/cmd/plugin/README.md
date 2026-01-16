# Secret Reader plugin

When executed by a controller, this plugin reads the `token` from the Kubernetes Secret `<CONSUMER_NAMESPACE>/<CLUSTER_PROFILE_NAME>` and writes an ExecCredential (JSON) to stdout.

See also:

- Controller example: [`examples/controller-example/plugins/secretreader/README.md`](../../../../examples/controller-example/plugins/secretreader/README.md)

The specification follows the Secret Reader plugin KEP.

## Required RBAC

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: secretreader
  namespace: <CONSUMER_NAMESPACE>
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: secretreader
  namespace: <CONSUMER_NAMESPACE>
subjects:
- kind: ServiceAccount
  name: <CONSUMER_SERVICE_ACCOUNT_NAME>
  namespace: <CONSUMER_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: secretreader
```

## Build

```bash
make build-secretreader-plugin
```

## Usage in a controller

Use the following provider config to exec the secret-reader plugin.

```jsonc
{
  "providers": [
    {
      "name": "secretreader",
      "execConfig": {
        "apiVersion": "client.authentication.k8s.io/v1",
        "command": "./bin/secretreader-plugin",
        "provideClusterInfo": true
      }
    }
  ]
}
```

### Note: `ClusterProfile.status.accessProviders[].cluster.extensions`

- Required: set `extensions[].name` to `client.authentication.k8s.io/exec`.
- The library reads only the `extension` field of that entry and passes it through to `ExecCredential.Spec.Cluster.Config`.
- The `secretreader` plugin uses `clusterName` inside that Config.

Example:

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
