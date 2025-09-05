# Secret Reader plugin

When executed by a controller, this plugin reads the `token` from the Kubernetes Secret `<CONSUMER_NAMESPACE>/<CLUSTER_PROFILE_NAME>` and writes an ExecCredential (JSON) to stdout.

The specification follows the Secret Reader plugin KEP.

## Required RBAC (example)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: secretreader-clusterprofiles
rules:
- apiGroups: ["multicluster.x-k8s.io"]
  resources: ["clusterprofiles"]
  verbs: ["list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: secretreader-clusterprofiles
subjects:
- kind: ServiceAccount
  name: <CONSUMER_SERVICE_ACCOUNT_NAME>
  namespace: <CONSUMER_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: secretreader-clusterprofiles
---
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
go build -o ./bin/secretreader-plugin ./cmd/secretreader-plugin
```

## Usage in a controller

```jsonc
{
  "providers": [
    {
      "name": "secretreader",
      "execConfig": {
        "apiVersion": "client.authentication.k8s.io/v1beta1",
        "command": "./bin/secretreader-plugin",
        "provideClusterInfo": true
      }
    }
  ]
}
```
