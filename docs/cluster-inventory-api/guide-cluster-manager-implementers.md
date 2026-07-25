# Guide for Cluster Manager Implementers

This is a step-by-step guide for building your own cluster manager that publishes `ClusterProfile` objects for the clusters it manages. By the end, you will have a working local Kind setup and a Go program that creates `ClusterProfile` objects with health conditions, properties, and version information — ready to be consumed by tools like [Kueue](https://kueue.sigs.k8s.io/), [Knative Operator](https://knative.dev/), or [multicluster-runtime](https://github.com/kubernetes-sigs/multicluster-runtime).

For more information on existing cluster manager implementations, see: [Cluster Inventory API Implementations](./index.md#cluster-managers). If you are building a consumer instead, see the [Guide for ClusterProfile Consumers](./guide-cluster-profile-consumers.md). For projects migrating from proprietary cluster registry APIs, see the [Migration Notes](./migration-notes.md).

**What you will build:** A Go program that creates a `ClusterProfile` and populates its status — the same pattern your production controller would use.

**Prerequisites**

- [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/) — for creating a local Kubernetes cluster.
- `kubectl` — configured and on your `$PATH`.
- `Go` 1.21+ (preferably 1.25+).

The final working code for this guide lives in [`examples/cluster-manager-example/`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/cluster-manager-example).

---

## Step 1: Create a Local Kind Cluster

Create a Kind cluster to use as the hub where `ClusterProfile` objects will live:

```bash
kind create cluster --name hub
kubectl config use-context kind-hub
```

If you want to also simulate spoke (member) clusters for testing access providers later, create them now:

```bash
kind create cluster --name spoke-1
kind create cluster --name spoke-2
kubectl config use-context kind-hub
```

> **Tip:** The [`examples/cluster-manager-example/`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/cluster-manager-example) directory includes a `setup-kind.sh` script that automates hub cluster creation, CRD installation, and namespace setup:
>
> ```bash
> bash ./examples/cluster-manager-example/setup-kind.sh
> ```

---

## Step 2: Install the ClusterProfile CRD

The `ClusterProfile` CRD must be installed on the hub cluster before any controller can create or read profile objects.

If you have cloned the [cluster-inventory-api repo](https://github.com/kubernetes-sigs/cluster-inventory-api) locally:

```bash
kubectl apply -f config/crd/bases/multicluster.x-k8s.io_clusterprofiles.yaml
```

Otherwise, install directly from the upstream repository:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-inventory-api/main/config/crd/bases/multicluster.x-k8s.io_clusterprofiles.yaml
```

Verify the CRD is installed:

```bash
kubectl get crd clusterprofiles.multicluster.x-k8s.io
```

Expected output:

```
NAME                                      CREATED AT
clusterprofiles.multicluster.x-k8s.io    2025-01-01T00:00:00Z
```

---

## Step 3: Import the Go Module

Create a new Go module for your cluster manager, or add the dependency to an existing project:

```bash
mkdir cluster-manager && cd cluster-manager
go mod init example.com/cluster-manager
go get sigs.k8s.io/cluster-inventory-api@latest
go mod tidy
```

The module provides three key packages:

| Package | Purpose |
|---------|---------|
| `sigs.k8s.io/cluster-inventory-api/apis/v1alpha1` | Go types for `ClusterProfile` and related structs |
| `sigs.k8s.io/cluster-inventory-api/client/clientset/versioned` | Generated typed clientset (`ciaclient`) |
| `sigs.k8s.io/cluster-inventory-api/pkg/access` | Consumer-side library for building `rest.Config` from a profile |

The generated typed clientset provides Go functions to interact with `ClusterProfile` resources:

- `Create()` — publish a new profile.
- `Update()` / `UpdateStatus()` — modify the spec or status.
- `Get()` / `List()` — fetch profiles.
- `Watch()` — listen for events in real-time.

In your controller's `main.go`, import and initialize the typed client:

```go
import (
    "path/filepath"

    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"
    ciaclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

// Build a rest.Config from your kubeconfig
kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
hubConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
if err != nil {
    log.Fatalf("failed to build kubeconfig: %v", err)
}

// Create the cluster-inventory-api typed client
cic, err := ciaclient.NewForConfig(hubConfig)
if err != nil {
    log.Fatalf("failed to construct cluster-inventory client: %v", err)
}
```

---

## Step 4: Choose a Namespace Strategy

`ClusterProfile` objects are **namespace-scoped**. The namespace you choose should reflect your deployment model:

| Strategy | When to use |
|----------|-------------|
| **Single namespace** (e.g. `cluster-inventory`) | Simple deployments; all clusters in one place. |
| **Per-cluster-manager namespace** | Multiple cluster managers on one hub; each manager owns its namespace. |
| **Namespace = ClusterSet** | The namespace carries the `multicluster.x-k8s.io/clusterset=<name>` label to group clusters into a ClusterSet. |

For this guide, we will use a single namespace:

```bash
kubectl create namespace cluster-inventory
```

To mark a namespace as belonging to a ClusterSet:

```bash
kubectl label namespace cluster-inventory \
  multicluster.x-k8s.io/clusterset=my-fleet
```

---

## Step 5: Create a ClusterProfile

The only required field in `spec` is `clusterManager.name`. This field is **immutable** after creation, so choose a stable, unique name for your cluster manager instance.

You must also add the `x-k8s.io/cluster-manager` label with the same value to make filtering easier for consumers.

### YAML example

Save this as `clusterprofile.yaml` (also available in [`examples/cluster-manager-example/clusterprofile.yaml`](https://github.com/kubernetes-sigs/cluster-inventory-api/blob/main/examples/cluster-manager-example/clusterprofile.yaml)):

```yaml
apiVersion: multicluster.x-k8s.io/v1alpha1
kind: ClusterProfile
metadata:
  name: cluster-us-west-1
  namespace: cluster-inventory
  labels:
    x-k8s.io/cluster-manager: my-fleet-manager
spec:
  displayName: "US West 1 Production Cluster"
  clusterManager:
    name: my-fleet-manager
```

Apply it and verify:

```bash
kubectl apply -f clusterprofile.yaml
kubectl get clusterprofiles -n cluster-inventory
```

Expected output:

```
NAME                 AGE
cluster-us-west-1    5s
```

### Go example (in your reconciler)

The equivalent in Go code — this is what your controller's reconcile loop would do. The `cic` client is the typed client you created in [Step 3](#step-3-import-the-go-module). A complete runnable version of this code is in [`examples/cluster-manager-example/main.go`](https://github.com/kubernetes-sigs/cluster-inventory-api/blob/main/examples/cluster-manager-example/main.go).

> **Try it out:** To run this code end-to-end against a local Kind cluster and see how it creates a `ClusterProfile` and updates its status, follow the instructions in the [examples/cluster-manager-example README](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/cluster-manager-example).

```go
import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

cp := &v1alpha1.ClusterProfile{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "cluster-us-west-1",
        Namespace: "cluster-inventory",
        Labels: map[string]string{
            v1alpha1.LabelClusterManagerKey: "my-fleet-manager",
        },
    },
    Spec: v1alpha1.ClusterProfileSpec{
        DisplayName: "US West 1 Production Cluster",
        ClusterManager: v1alpha1.ClusterManager{
            Name: "my-fleet-manager",
        },
    },
}

createdCP, err := cic.ApisV1alpha1().ClusterProfiles("cluster-inventory").Create(
    ctx, cp, metav1.CreateOptions{},
)
if err != nil {
    log.Fatalf("failed to create ClusterProfile: %v", err)
}
log.Printf("Created ClusterProfile: %s", createdCP.Name)
```

---

## Step 6: Maintain the Status

Status is the primary mechanism for communicating cluster health, version, properties, and access information to consumers. Your controller should update these fields whenever the underlying cluster state changes.

In Kubernetes, Status is modeled as a **subresource**. To update it, you call `UpdateStatus()` (not `Update()`). This separation provides security: users or pipelines might have permissions to edit the Spec, but only the controller has permissions to edit the Status.

### 6.1 Conditions

Kubernetes uses a list of `Condition` objects to represent status aspects. For `ClusterProfile`, the key condition is `ControlPlaneHealthy` (constant `v1alpha1.ClusterConditionControlPlaneHealthy`).

Use the standard library function `meta.SetStatusCondition()` to safely add or update conditions:

```go
import (
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

// Set ControlPlaneHealthy = True
meta.SetStatusCondition(&createdCP.Status.Conditions, metav1.Condition{
    Type:               v1alpha1.ClusterConditionControlPlaneHealthy,
    Status:             metav1.ConditionTrue,
    Reason:             "ClusterReachable",
    Message:            "API server is reachable and responding",
    LastTransitionTime: metav1.Now(),
})
```

If your health probe fails, set `Status: metav1.ConditionFalse` with an appropriate `Reason` and `Message`.

### 6.2 Kubernetes Version

A simple struct tracking the Kubernetes version of the member cluster:

```go
createdCP.Status.Version = v1alpha1.ClusterVersion{
    Kubernetes: "1.32.0",
}
```

### 6.3 Properties

Properties let consumers make scheduling and placement decisions without querying individual member clusters. Well-known property names are defined in [KEP-2149 (ClusterProperty)](https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/2149-clusterid); any custom name is also valid:

| Property name | Description |
|---------------|-------------|
| `clusterset.k8s.io/cluster-id` | Stable, unique cluster identifier |
| `topology.kubernetes.io/region` | Cloud region (e.g. `us-west-2`) |
| `topology.kubernetes.io/zone` | Availability zone (e.g. `us-west-2a`) |

```go
import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

createdCP.Status.Properties = []v1alpha1.Property{
    {
        Name:             "clusterset.k8s.io/cluster-id",
        Value:            "a1b2c3d4-e5f6-...",
        LastObservedTime: metav1.Now(),
    },
    {
        Name:             "topology.kubernetes.io/region",
        Value:            "us-west-2",
        LastObservedTime: metav1.Now(),
    },
    {
        Name:             "topology.kubernetes.io/zone",
        Value:            "us-west-2a",
        LastObservedTime: metav1.Now(),
    },
    // Custom property (any name is valid)
    {
        Name:             "my-manager.io/tier",
        Value:            "production",
        LastObservedTime: metav1.Now(),
    },
}
```

### 6.4 Calling UpdateStatus

Once the fields are set on the local struct, persist them via the status subresource:

```go
updatedCP, err := cic.ApisV1alpha1().ClusterProfiles("cluster-inventory").UpdateStatus(
    ctx, createdCP, metav1.UpdateOptions{},
)
if err != nil {
    log.Fatalf("failed to update ClusterProfile status: %v", err)
}
log.Printf("Status updated — conditions: %d, version: %s, properties: %d",
    len(updatedCP.Status.Conditions),
    updatedCP.Status.Version.Kubernetes,
    len(updatedCP.Status.Properties),
)
```

### Using `kubectl` to patch status

For quick testing, you can also set status fields directly with `kubectl patch --subresource=status`:

```bash
kubectl patch clusterprofile cluster-us-west-1 \
  -n cluster-inventory \
  --type=merge \
  --subresource=status \
  -p '{
    "status": {
      "conditions": [
        {
          "type": "ControlPlaneHealthy",
          "status": "True",
          "reason": "ClusterReachable",
          "message": "API server is reachable and responding",
          "lastTransitionTime": "2025-01-01T00:00:00Z"
        }
      ],
      "version": {"kubernetes": "1.32.0"},
      "properties": [
        {"name": "topology.kubernetes.io/region", "value": "us-west-1"},
        {"name": "topology.kubernetes.io/zone", "value": "us-west-1a"}
      ]
    }
  }'
```

---

## Step 7: Access Providers (KEP-5339)

`status.accessProviders` is how consumers learn how to connect to a member cluster. Each entry names an access mechanism and provides the cluster connection details — server URL, CA certificate data, and optional exec-plugin extensions.

```go
import (
    clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

createdCP.Status.AccessProviders = []v1alpha1.AccessProvider{
    {
        // Name must match a provider entry in the consumer's provider config file
        Name: "secretreader",
        Cluster: clientcmdv1.Cluster{
            Server:                   "https://api.cluster-us-west-1.example.com:6443",
            CertificateAuthorityData: caBundle, // []byte, base64-decoded PEM
            // Pass cluster-specific data to the exec plugin via the reserved extension
            Extensions: []clientcmdv1.NamedExtension{
                {
                    // This extension key is defined by the client-go exec credential API (KEP-541); KEP-5339 reuses it
                    Name: "client.authentication.k8s.io/exec",
                    Extension: runtime.RawExtension{
                        // The exec plugin reads this JSON from ExecCredential.Spec.Cluster.Config
                        Raw: []byte(`{"clusterName":"cluster-us-west-1"}`),
                    },
                },
            },
        },
    },
}
```

---

## Step 8: Credential Plugin Setup

The exec credential plugin mechanism (KEP-5339) lets consumers obtain cluster-specific tokens without embedding credentials in the `ClusterProfile`. The `cluster-inventory-api` repository ships two built-in plugins.

### Built-in plugins

| Plugin | What it does |
| -------- | ------------- |
| `secretreader` | Reads a bearer token from a Kubernetes `Secret` on the hub cluster. The secret name is taken from `clusterName` in the exec config extension. |
| `kubeconfig-secretreader` | Reads a full kubeconfig from a `Secret` and uses it to authenticate. Useful when member clusters use short-lived OIDC tokens. |

Build and install the `secretreader` binary:

```bash
# From the cluster-inventory-api repository root:
make build-secretreader-plugin
# Binary is placed at ./bin/secretreader-plugin
```

Or install it via `go install`:

```bash
go install sigs.k8s.io/cluster-inventory-api/plugins/secretreader/cmd/plugin@latest
# The binary is named "plugin"; rename or symlink it to "secretreader"
```

### Writing a provider config file

As a cluster manager, you need to provide a **provider config file** to your consumers. This JSON file tells the consumer-side `access` library ([`pkg/access`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/pkg/access)) which exec credential plugin to invoke when authenticating to a member cluster. Consumers load this file via `access.NewFromFile()` or the `--clusterprofile-provider-file` CLI flag.

Here is an example using the `secretreader` plugin:

```json
{
  "providers": [
    {
      "name": "secretreader",
      "execConfig": {
        "apiVersion": "client.authentication.k8s.io/v1",
        "command": "secretreader",
        "args": [],
        "provideClusterInfo": true,
        "interactiveMode": "Never"
      },
      "profileSourcedCLIArgsPolicy": "Append",
      "profileSourcedEnvVarsPolicy": "AppendIfNotExists"
    }
  ]
}
```

**Key fields:**

| Field | Description |
| ----- | ----------- |
| `name` | **Must exactly match** the `name` in `ClusterProfile.status.accessProviders[]`. This is how the library picks the right plugin for a given profile. |
| `execConfig.command` | Path to the credential plugin binary (e.g. `secretreader`, `./bin/secretreader-plugin`). |
| `execConfig.provideClusterInfo` | When `true`, the library passes cluster information (server, CA) to the plugin via `ExecCredential.Spec.Cluster`. |
| `profileSourcedCLIArgsPolicy` | `"Append"` lets the `ClusterProfile` inject additional CLI args into the plugin command. `"Ignore"` disables this. |
| `profileSourcedEnvVarsPolicy` | `"AppendIfNotExists"` lets the `ClusterProfile` inject environment variables. `"Replace"` overwrites existing ones. `"Ignore"` disables this. |

**How consumers use it:** The consumer calls `access.NewFromFile("provider-config.json")` and then `cfg.BuildConfigFromCP(clusterProfile)` — the library matches the provider name, invokes the exec plugin, and returns a `*rest.Config` ready for `client-go`. See the [Guide for ClusterProfile Consumers](./guide-cluster-profile-consumers.md) for the consumer side.

To try it end-to-end with a hub and spoke cluster, see the [`examples/controller-example/`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/controller-example) which includes setup scripts, a provider config, and a consumer program that connects to a spoke cluster and lists pods.

---

## Step 9: Reference Implementations

For complete working examples, see:

- **[`examples/cluster-manager-example/`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/cluster-manager-example)** — the companion code for this guide. Creates a `ClusterProfile` with status (conditions, version, properties) on a Kind cluster.
- **[`examples/controller-example/`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/controller-example)** — a consumer controller that reads a `ClusterProfile` and connects to the spoke cluster using the `access` package. Includes demo scripts for the `secretreader` and `kubeconfig-secretreader` plugins.
- **[OCM's hub-side ClusterProfile reconciler](https://github.com/open-cluster-management-io/ocm/tree/main/pkg/registration/hub/clusterprofile)** — a production-grade example that syncs `ManagedCluster` objects to `ClusterProfile` objects, maintained by the Open Cluster Management project.

For projects migrating from proprietary cluster registry APIs (OCM, Karmada, GKE Fleet), see the [Migration Notes](./migration-notes.md).