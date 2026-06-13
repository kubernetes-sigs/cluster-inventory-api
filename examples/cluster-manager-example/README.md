## Cluster Manager Example

A minimal Go program that demonstrates how a **cluster manager** creates and maintains `ClusterProfile` objects using the Cluster Inventory API.

This example is the companion code for the [Guide for Cluster Managers](https://multicluster.sigs.k8s.io/implementations/cluster-inventory-api-implementation/guide-cluster-manager-implementers/).

### What It Does

1. Connects to a hub cluster via kubeconfig
2. Creates a `ClusterProfile` object with:
   - The `x-k8s.io/cluster-manager` label for filtering
   - A human-readable display name
   - The immutable `clusterManager.name` field
3. Updates the `ClusterProfile` status with:
   - `ControlPlaneHealthy` condition
   - Kubernetes version
   - Cluster properties (region, zone, cluster-id)

### Prerequisites

- `kind` and `kubectl` installed
- `go` 1.21+ installed
- Working directory is the repository root

### Quick Start: Try It End-to-End on Kind

#### 1. Set up the environment

Run the setup script from the root of the repository. This creates a Kind hub cluster, installs the ClusterProfile CRD, and creates the `cluster-inventory` namespace.

```bash
bash ./examples/cluster-manager-example/setup-kind.sh
```

#### 2. Run the example

Download the module dependencies, then run the program:

```bash
cd examples/cluster-manager-example
go mod tidy
go run main.go
```

Expected output:

```bash
Creating ClusterProfile cluster-us-west-1...
Successfully created ClusterProfile: cluster-us-west-1
Updating ClusterProfile status...
Status updated for ClusterProfile: cluster-us-west-1
  Conditions: 1 set
  Version: 1.32.0
  Properties: 2 set
```

#### 3. Verify on the cluster

```bash
kubectl get clusterprofiles -n cluster-inventory
```

```bash
NAME                 AGE
cluster-us-west-1    10s
```

Inspect the full object to see all spec and status fields:

```bash
kubectl get clusterprofile cluster-us-west-1 -n cluster-inventory -o yaml
```

You should see the `spec` with `clusterManager.name: my-fleet-manager` and the `status` populated with conditions, version, and properties.

#### 4. Cleanup

```bash
bash ./examples/cluster-manager-example/down.sh
```

---

### Applying via YAML (Alternative)

If you prefer declarative YAML over Go code, you can apply the ClusterProfile directly:

```bash
kubectl apply -f ./examples/cluster-manager-example/clusterprofile.yaml
```

Note that `kubectl apply` does not set status fields because status is a subresource. To populate `status` (conditions, properties, accessProviders), you need to use `kubectl patch --subresource=status` or a Go controller (as shown in `main.go`). 

You can manually test patching the status via:

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

### Files

| File | Description |
|------|-------------|
| `main.go` | Go program that creates a ClusterProfile and updates its status |
| `clusterprofile.yaml` | Equivalent YAML manifest for creating the same ClusterProfile |
| `setup-kind.sh` | Creates a Kind hub cluster and installs the CRD |
| `down.sh` | Tears down the Kind cluster |
