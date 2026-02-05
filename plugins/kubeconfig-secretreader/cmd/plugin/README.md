# Kubeconfig Secret Reader plugin

When executed by a controller, this plugin reads a kubeconfig from a Kubernetes Secret `<NAMESPACE>/<SECRET_NAME>` (from `data[<KEY>]`) and extracts authentication credentials from the specified kubeconfig context (or `current-context` if not specified).

See also:

- Controller example: [`examples/controller-example/plugins/kubeconfig-secretreader/README.md`](../../../../examples/controller-example/plugins/kubeconfig-secretreader/README.md)

It supports:

- token-based authentication (`users[].user.token`), and/or
- certificate-based authentication (`users[].user.client-certificate-data` and `users[].user.client-key-data`)

It then writes a **minimal** `ExecCredential` (JSON) containing only `apiVersion`, `kind`, and `status` to stdout.

## Support matrix

| Feature | Status | Config field | Notes |
|--------|--------|--------------|-------|
| Secret name | Supported | `name` (required) | Set by cluster manager in `accessProviders[].cluster.extensions` |
| Secret namespace | Supported | `namespace` (optional) | Omitted → inferred (kubeconfig context → in-cluster namespace file → `default`) |
| Secret data key | Supported | `key` (required) | Key in `Secret.data` holding the kubeconfig |
| Kubeconfig context | Supported | `context` (optional) | Omitted → `current-context` |
| Token auth | Supported | — | `users[].user.token` |
| Client cert/key (inline) | Supported | — | `client-certificate-data` + `client-key-data` only; output as PEM |
| Client cert/key (file path) | Not supported | — | Use `*-data` in kubeconfig |
| Username/password | Not supported | — | Not implemented |
| TokenFile | Not supported | — | Not implemented |
| Kubeconfig `extensions` | Not supported | — | Plugin rejects kubeconfigs that use extensions |
| CA/key/cert in separate Secret keys | Not supported | — | Only inline `*-data` in the kubeconfig |
| Kubeconfig `user.exec` | Not supported | — | Use exec in `accessProviders` instead; see [Security considerations](#security-considerations) |

## Required RBAC

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubeconfig-secretreader
  namespace: <CONSUMER_NAMESPACE>
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kubeconfig-secretreader
  namespace: <CONSUMER_NAMESPACE>
subjects:
- kind: ServiceAccount
  name: <CONSUMER_SERVICE_ACCOUNT_NAME>
  namespace: <CONSUMER_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: kubeconfig-secretreader
```

## Build

```bash
make build-kubeconfig-secretreader-plugin
```

## Usage in a controller

Use the following provider config to exec the kubeconfig-secretreader plugin.

```jsonc
{
  "providers": [
    {
      "name": "kubeconfig-secretreader",
      "execConfig": {
        "apiVersion": "client.authentication.k8s.io/v1",
        "command": "./bin/kubeconfig-secretreader-plugin",
        "provideClusterInfo": true
      }
    }
  ]
}
```

### Note: `ClusterProfile.status.accessProviders[].cluster.extensions`

- Required: set `extensions[].name` to `client.authentication.k8s.io/exec`.
- The library reads only the `extension` field of that entry and passes it through to `ExecCredential.Spec.Cluster.Config`.
- The `kubeconfig-secretreader` plugin uses `name`, `key`, `namespace` (optional), and `context` (optional) inside that Config.
- `extension.name` is the **Secret name** to read.
- If `extension.namespace` is omitted, the plugin uses an inferred namespace (kubeconfig current-context namespace → in-cluster service account namespace file → `"default"`).

Example:

```yaml
status:
  accessProviders:
  - name: kubeconfig-secretreader
    cluster:
      server: https://<spoke-server>
      certificate-authority-data: <BASE64_CA>
      extensions:
      - name: client.authentication.k8s.io/exec
        extension:
          name: docker-test-kubeconfig   # Secret metadata.name (required)
          key: value                     # Secret.data key (required)
          namespace: default              # Optional: Secret namespace (defaults to inferred namespace)
          context: docker-test-admin@docker-test-k0s  # Optional: kubeconfig context name (defaults to current-context)
```

## Secret Format

The Secret must contain a kubeconfig YAML in the specified key.

The kubeconfig must include at least one authentication method in the selected user:

- `token` (for token-based authentication), and/or
- `client-certificate-data` and `client-key-data` (for certificate-based authentication)

Both authentication methods can be present, but at least one must be available.

Notes:

- `client-certificate-data` / `client-key-data` in kubeconfig are **base64-encoded**. This plugin loads the kubeconfig via `client-go`, which **decodes** them and outputs **PEM text** in `ExecCredential.status.clientCertificateData/clientKeyData`.
- File-path based fields (`client-certificate`, `client-key`) are **not supported** by this plugin; use `*-data` fields.
- Kubeconfig `extensions` are **not supported** by this plugin.

Example Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: docker-test-kubeconfig
  namespace: default
stringData:
  value: |
    apiVersion: v1
    kind: Config
    clusters:
    - cluster:
        certificate-authority-data: <BASE64_CA>
        server: https://10.244.0.14:30443
      name: docker-test-k0s
    contexts:
    - context:
        cluster: docker-test-k0s
        user: docker-test-admin
      name: docker-test-admin@docker-test-k0s
    current-context: docker-test-admin@docker-test-k0s
    users:
    - name: docker-test-admin
      user:
        client-certificate-data: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCg=="
        client-key-data: "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQo="
```

## Plugin Output

The plugin returns a `ExecCredential` with the authentication credentials found in the kubeconfig:

**For token-based authentication:**

```json
{
  "apiVersion": "client.authentication.k8s.io/v1",
  "kind": "ExecCredential",
  "status": {
    "token": "eyJhbGciOiJSUzI1NiIs..."
  }
}
```

**For certificate-based authentication:**

```json
{
  "apiVersion": "client.authentication.k8s.io/v1",
  "kind": "ExecCredential",
  "status": {
    "clientCertificateData": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
    "clientKeyData": "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
  }
}
```

**If both are present in the kubeconfig, both will be included in the response:**

```json
{
  "apiVersion": "client.authentication.k8s.io/v1",
  "kind": "ExecCredential",
  "status": {
    "token": "eyJhbGciOiJSUzI1NiIs...",
    "clientCertificateData": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
    "clientKeyData": "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
  }
}
```

## Security considerations

- This plugin **only reads** a Secret, parses the kubeconfig **statically**, and outputs an `ExecCredential`. It does not execute any binary from the kubeconfig.
- **Kubeconfig `user.exec` is not supported.** If exec-based authentication is needed, configure it in the cluster manager’s `accessProviders` (exec plugin) so that execution and lifecycle are explicit and auditable.
