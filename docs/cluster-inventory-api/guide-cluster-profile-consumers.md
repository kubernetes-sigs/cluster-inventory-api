# Guide for ClusterProfile Consumers

This is a step-by-step guide for building a controller, scheduler, or tool that **consumes** `ClusterProfile` objects published by a cluster manager. By the end, you will have a working local Kind setup and a Go program that discovers registered clusters, inspects their properties, and connects to a member cluster — the same patterns used by [Kueue](https://kueue.sigs.k8s.io/), [Knative Operator](https://knative.dev/), [multicluster-runtime](https://github.com/kubernetes-sigs/multicluster-runtime), and [Argo CD](https://argo-cd.readthedocs.io/).

For more information on existing consumer implementations, see: [Cluster Inventory API Implementations](./index.md#clusterprofile-api-consumers).

**What you will build:** A Go program that lists `ClusterProfile` objects on a hub cluster, checks their health, and connects to a member cluster to list pods.

**Prerequisites**

- [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/) — for creating local Kubernetes clusters.
- `kubectl` — configured and on your `$PATH`.
- `Go` 1.21+ (preferably 1.25+).
- A hub cluster with at least one `ClusterProfile` already published (see [Step 1](#step-1-set-up-your-environment)).

The final working code for this guide lives in [`examples/controller-example/`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/controller-example).

---

## Step 1: Set Up Your Environment

If you followed the [Guide for Cluster Manager Implementers](./guide-cluster-manager-implementers.md), you already have a `kind-hub` cluster with the CRD installed and a `ClusterProfile` created. You can skip to [Step 2](#step-2-import-the-go-module).

Otherwise, bootstrap a hub cluster with a sample `ClusterProfile` using the provided scripts:

```bash
# Create a Kind cluster named "hub" and install the ClusterProfile CRD
bash ./examples/cluster-manager-example/setup-kind.sh

# Run the cluster-manager example to create a sample ClusterProfile with status
go run ./examples/cluster-manager-example/...
```

Verify the profile exists:

```bash
kubectl get clusterprofiles -n cluster-inventory
```

Expected output:

```
NAME                 AGE
cluster-us-west-1    5s
```

If you also want to test connecting to a member cluster later (Steps 6–7), create spoke clusters now:

```bash
kind create cluster --name spoke-1
kind create cluster --name spoke-2
kubectl config use-context kind-hub
```

---

## Step 2: Import the Go Module

Create a new Go module for your consumer, or add the dependency to an existing project:

```bash
mkdir cluster-consumer && cd cluster-consumer
go mod init example.com/cluster-consumer
go get sigs.k8s.io/cluster-inventory-api@latest
go mod tidy
```

The module provides three key packages:

| Package | Purpose |
|---------|---------|
| `sigs.k8s.io/cluster-inventory-api/apis/v1alpha1` | Go types for `ClusterProfile` and related structs |
| `sigs.k8s.io/cluster-inventory-api/client/clientset/versioned` | Generated typed clientset (`ciaclient`) |
| `sigs.k8s.io/cluster-inventory-api/pkg/access` | Consumer-side library for building `rest.Config` from a profile |

In your consumer's `main.go`, import and initialize the typed client:

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

## Step 3: List and Watch ClusterProfiles

### List all profiles across namespaces

Pass an empty namespace (`""`) to list `ClusterProfile` objects across all namespaces:

```go
import (
    "context"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ciaclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

cic, err := ciaclient.NewForConfig(hubConfig)
if err != nil {
    return fmt.Errorf("failed to create cluster-inventory client: %w", err)
}

// List all ClusterProfiles across all namespaces
profiles, err := cic.ApisV1alpha1().ClusterProfiles("").List(
    ctx, metav1.ListOptions{},
)
```

### Filter by cluster manager label

Use the `x-k8s.io/cluster-manager` label to scope your query to profiles from a specific cluster manager:

```go
import "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"

profiles, err := cic.ApisV1alpha1().ClusterProfiles("").List(
    ctx,
    metav1.ListOptions{
        LabelSelector: v1alpha1.LabelClusterManagerKey + "=my-fleet-manager",
    },
)
```

### Watch for changes (event-driven)

Use `Watch()` to react to cluster additions, updates, and removals in real-time:

```go
watcher, err := cic.ApisV1alpha1().ClusterProfiles("").Watch(
    ctx,
    metav1.ListOptions{
        LabelSelector: v1alpha1.LabelClusterManagerKey + "=my-fleet-manager",
    },
)
if err != nil {
    return err
}
defer watcher.Stop()

for event := range watcher.ResultChan() {
    cp, ok := event.Object.(*v1alpha1.ClusterProfile)
    if !ok {
        continue
    }
    log.Printf("event=%s cluster=%s/%s", event.Type, cp.Namespace, cp.Name)
}
```

---

## Step 4: Read Properties for Scheduling Decisions

`status.properties` lets you make placement and scheduling decisions without querying individual member clusters. Iterate over the property list and look up named properties:

```go
func getProperty(cp *v1alpha1.ClusterProfile, name string) (string, bool) {
    for _, p := range cp.Status.Properties {
        if p.Name == name {
            return p.Value, true
        }
    }
    return "", false
}

// Example: prefer clusters in a given region
for _, cp := range profiles.Items {
    region, ok := getProperty(&cp, "topology.kubernetes.io/region")
    if !ok || region != "us-west-2" {
        continue
    }
    // This cluster is in us-west-2 — schedule here
}
```

Well-known [KEP-2149 (ClusterProperty)](https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/2149-clusterid) property names:

| Property name | Description |
|---------------|-------------|
| `clusterset.k8s.io/cluster-id` | Stable, unique cluster identifier |
| `topology.kubernetes.io/region` | Cloud region (e.g. `us-west-2`) |
| `topology.kubernetes.io/zone` | Availability zone (e.g. `us-west-2a`) |

---

## Step 5: Check Cluster Health

Always check `status.conditions` before attempting to build a config or dispatch workloads. The key condition is `ControlPlaneHealthy` (constant `v1alpha1.ClusterConditionControlPlaneHealthy`).

```go
import (
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

func isHealthy(cp *v1alpha1.ClusterProfile) bool {
    cond := meta.FindStatusCondition(cp.Status.Conditions, v1alpha1.ClusterConditionControlPlaneHealthy)
    return cond != nil && cond.Status == metav1.ConditionTrue
}
```

Use this in your scheduling or dispatch loop:

```go
for _, cp := range profiles.Items {
    if !isHealthy(&cp) {
        log.Printf("Skipping unhealthy cluster %s/%s", cp.Namespace, cp.Name)
        continue
    }
    // Safe to connect and dispatch workloads
}
```

---

## Step 6: Connect to a Member Cluster

The [`pkg/access`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/pkg/access) package is the standard way to build a `*rest.Config` from a `ClusterProfile`. It reads a **provider config file** to find the right exec plugin and then populates the config using `status.accessProviders`.

### Load the provider config

```go
import "sigs.k8s.io/cluster-inventory-api/pkg/access"

// Path is typically passed as a flag: --clusterprofile-provider-file
accessCfg, err := access.NewFromFile("clusterprofile-provider-file.json")
if err != nil {
    return fmt.Errorf("failed to load access config: %w", err)
}
```

Or register the standard flag and parse:

```go
providerFile := access.SetupProviderFileFlag() // registers --clusterprofile-provider-file
flag.Parse()

accessCfg, err := access.NewFromFile(*providerFile)
```

### Build a `rest.Config` for a specific cluster

```go
spokeConfig, err := accessCfg.BuildConfigFromCP(cp)
if err != nil {
    return fmt.Errorf("failed to build config for %s: %w", cp.Name, err)
}
```

`BuildConfigFromCP` performs the following steps internally:
1. Matches a provider in your config file against `cp.Status.AccessProviders` by name.
2. Extracts the `server`, `certificateAuthorityData`, and optional `proxyURL` from the matching `AccessProvider.Cluster`.
3. Reads exec-plugin extensions (`client.authentication.k8s.io/exec`, per KEP-5339) and merges them into the `ExecProvider` config.
4. Returns a fully populated `*rest.Config`.

### Use the config with any Kubernetes client

```go
// client-go
import kubernetes "k8s.io/client-go/kubernetes"

mclient, err := kubernetes.NewForConfig(spokeConfig)
pods, err := mclient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

// controller-runtime
import crclient "sigs.k8s.io/controller-runtime/pkg/client"

crc, err := crclient.New(spokeConfig, crclient.Options{Scheme: scheme})
var podList corev1.PodList
err = crc.List(ctx, &podList)
```

---

## Step 7: Minimal End-to-End Example

The following is a condensed version of the canonical [`examples/controller-example/main.go`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/controller-example) from the upstream repository. It discovers one cluster by name and lists its pods.

> **Try it out:** To run this end-to-end with a hub and spoke Kind cluster, follow the instructions in the [examples/controller-example README](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/controller-example).

```go
package main

import (
    "context"
    "flag"
    "log"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    kubernetes "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
    ciaclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
    "sigs.k8s.io/cluster-inventory-api/pkg/access"
)

func main() {
    // Flags
    providerFile := access.SetupProviderFileFlag()
    namespace := flag.String("namespace", "cluster-inventory", "Namespace of ClusterProfile objects")
    profileName := flag.String("clusterprofile", "", "Name of the ClusterProfile to connect to (required)")
    flag.Parse()

    if *profileName == "" {
        log.Fatal("-clusterprofile is required")
    }

    // Load the provider config (exec plugin config)
    accessCfg, err := access.NewFromFile(*providerFile)
    if err != nil {
        log.Fatalf("failed to load provider config: %v", err)
    }

    // Build hub cluster config (in-cluster first, then kubeconfig fallback)
    hubConfig, err := rest.InClusterConfig()
    if err != nil {
        hubConfig, err = clientcmd.BuildConfigFromFlags("", clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename())
        if err != nil {
            log.Fatalf("failed to load hub config: %v", err)
        }
    }

    // Fetch the ClusterProfile from the hub
    cic, err := ciaclient.NewForConfig(hubConfig)
    if err != nil {
        log.Fatalf("failed to create cluster-inventory client: %v", err)
    }
    cp, err := cic.ApisV1alpha1().ClusterProfiles(*namespace).Get(
        context.Background(), *profileName, metav1.GetOptions{},
    )
    if err != nil {
        log.Fatalf("failed to get ClusterProfile %s/%s: %v", *namespace, *profileName, err)
    }

    // Build a rest.Config for the spoke (member) cluster
    spokeConfig, err := accessCfg.BuildConfigFromCP(cp)
    if err != nil {
        log.Fatalf("failed to build spoke config: %v", err)
    }

    // Use the config to list pods on the spoke cluster
    mclient, err := kubernetes.NewForConfig(spokeConfig)
    if err != nil {
        log.Fatalf("failed to create spoke client: %v", err)
    }
    pods, err := mclient.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
    if err != nil {
        log.Fatalf("failed to list pods: %v", err)
    }
    log.Printf("Found %d pods on cluster %q", len(pods.Items), cp.Name)
}
```

Run it:

```bash
go run main.go \
  --clusterprofile-provider-file=./clusterprofile-provider-file.json \
  --namespace=cluster-inventory \
  --clusterprofile=cluster-us-west-1
```

---

## Step 8: Integration Patterns

### 8.1 Controller-runtime based controllers

[multicluster-runtime](https://github.com/kubernetes-sigs/multicluster-runtime) provides a `ClusterProfile` provider that automatically discovers clusters from `ClusterProfile` objects and wires them into a `controller-runtime` `Manager`. This is the recommended approach for building fleet-aware controllers.

```go
import (
    "sigs.k8s.io/multicluster-runtime/providers/cluster-inventory-api"
)

// The provider watches ClusterProfile objects and calls access.BuildConfigFromCP
// for each cluster as it is discovered.
provider := clusterprovider.New(accessCfg, client)
mgr.Add(provider)
```

See the [multicluster-runtime ClusterProfile provider](https://github.com/kubernetes-sigs/multicluster-runtime/tree/main/providers/cluster-inventory-api) and its [end-to-end example](https://github.com/kubernetes-sigs/multicluster-runtime/tree/main/examples/cluster-inventory-api) for a complete integration.

---

### 8.2 Batch schedulers (Kueue MultiKueue)

[Kueue](https://kueue.sigs.k8s.io/) v0.15+ supports referencing a `ClusterProfile` directly from a `MultiKueueCluster` resource. Enable the `MultiKueueClusterProfile` feature gate, then:

```yaml
apiVersion: kueue.x-k8s.io/v1alpha1
kind: MultiKueueCluster
metadata:
  name: worker-us-west-1
spec:
  workerRef:
    clusterProfile:
      name: cluster-us-west-1
      namespace: cluster-inventory
```

Kueue calls `access.BuildConfigFromCP()` internally using the provider config file you supply. See [Kueue MultiKueue setup](https://kueue.sigs.k8s.io/docs/tasks/manage/setup_multikueue/#configure-federated-credential-discovery-via-the-clusterprofile-api) for full configuration details.

---

### 8.3 GitOps tools (Argo CD)

The upstream Argo CD `ClusterProfile` integration ([PR #24509](https://github.com/argoproj/argo-cd/pull/24509), tracking [issue #24282](https://github.com/argoproj/argo-cd/issues/24282)) introduces a controller that watches `ClusterProfile` objects and creates a corresponding Argo CD cluster `Secret` for each, using `status.accessProviders` for connection details.

Once merged, it will support two credential modes:

1. **Exec plugin mode** (KEP-5339): Delegates to a configured exec plugin via the provider config file.
2. **Built-in cloud-provider shortcuts**: Direct credential resolution without a separate binary.

Until this is merged, users of GKE Fleet can use the [GKE Argo CD ClusterProfile Syncer](https://github.com/GoogleCloudPlatform/gke-fleet-management/tree/main/argocd-clusterprofile-syncer) as a stop-gap.

---

### 8.4 Serverless platforms (Knative Operator)

[Knative Operator](https://github.com/knative/operator) v1.22+ can deploy Knative Serving and Eventing to member clusters referenced by a `ClusterProfile`:

```yaml
apiVersion: operator.knative.dev/v1beta1
kind: KnativeServing
metadata:
  name: knative-serving
  namespace: knative-serving
spec:
  clusterProfileRef:
    name: cluster-us-west-1
    namespace: cluster-inventory
```

The operator calls `access.BuildConfigFromCP()` to connect and applies manifests to the target cluster. See the [Knative Operator multicluster guide](https://github.com/knative/operator/blob/main/docs/multicluster.md) for details.

---

## Step 9: Reference Implementations

For complete working examples, see:

- **[`examples/controller-example/`](https://github.com/kubernetes-sigs/cluster-inventory-api/tree/main/examples/controller-example)** — the companion code for this guide. Reads a `ClusterProfile` and connects to the spoke cluster using the `access` package. Includes demo scripts for the `secretreader` and `kubeconfig-secretreader` plugins.
- **[multicluster-runtime ClusterProfile provider](https://github.com/kubernetes-sigs/multicluster-runtime/tree/main/providers/cluster-inventory-api)** — a controller-runtime integration that discovers clusters from `ClusterProfile` objects.
- **[Kueue MultiKueue](https://kueue.sigs.k8s.io/docs/tasks/manage/setup_multikueue/)** — batch workload dispatch across clusters using `ClusterProfile` references.

For projects migrating from proprietary cluster registry APIs, see the [Migration Notes](./migration-notes.md).
