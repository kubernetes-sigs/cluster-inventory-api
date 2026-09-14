#!/bin/bash
set -euo pipefail

# This script sets up a local Kind demo environment for the Cluster Inventory API
# cluster-manager-example. It creates:
#   - A "hub" Kind cluster where ClusterProfile objects live
#   - The ClusterProfile CRD installed on the hub
#   - A "cluster-inventory" namespace for the ClusterProfile objects

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "${SCRIPT_DIR}/../.." && pwd)

echo "==> [1/4] Creating Kind cluster 'hub'..."
if kind get clusters 2>/dev/null | grep -q '^hub$'; then
  echo "    Kind cluster 'hub' already exists, skipping creation."
else
  kind create cluster --name hub
fi

echo "==> [2/4] Switching kubectl context to kind-hub..."
kubectl config use-context kind-hub

echo "==> [3/4] Installing ClusterProfile CRD on hub cluster..."
kubectl apply -f "${REPO_ROOT}/config/crd/bases/multicluster.x-k8s.io_clusterprofiles.yaml"

echo "==> [4/4] Creating 'cluster-inventory' namespace..."
kubectl create namespace cluster-inventory --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "=== Setup complete ==="
echo "Hub cluster: kind-hub"
echo "CRD installed: clusterprofiles.multicluster.x-k8s.io"
echo "Namespace: cluster-inventory"
echo ""
echo "Verify with:"
echo "  kubectl get crd clusterprofiles.multicluster.x-k8s.io"
echo "  kubectl get ns cluster-inventory"
