#!/bin/bash
set -euo pipefail

# This script sets up a demo with:
# - kind hub cluster and kind spoke cluster
# - a ServiceAccount on the spoke cluster that can list pods
# - a long-lived token for that ServiceAccount
# - a Secret on the hub cluster with data.token=<SA token>
# - the ClusterProfile CR on the hub cluster with secretreader provider pointing to the spoke cluster

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
EXAMPLE_DIR=$(cd "${SCRIPT_DIR}/../.." && pwd)
REPO_ROOT=$(cd "${EXAMPLE_DIR}/../.." && pwd)

echo "[1/9] Create spoke cluster"
kind create cluster --name "spoke"
# Wait for default ServiceAccount to be created in the default namespace on the spoke cluster
for i in {1..60}; do
  if kubectl --context "kind-spoke" -n default get sa default >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "[2/9] Create ServiceAccount, RBAC and token Secret on spoke cluster"
kubectl --context "kind-spoke" apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spoke-reader
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: pod-reader
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: spoke-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: pod-reader
subjects:
  - kind: ServiceAccount
    name: spoke-reader
    namespace: default
---
apiVersion: v1
kind: Secret
metadata:
  name: spoke-reader-token
  namespace: default
  annotations:
    kubernetes.io/service-account.name: spoke-reader
type: kubernetes.io/service-account-token
EOF

echo "[3/9] Create a sleep Pod on spoke cluster"
kubectl --context "kind-spoke" apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: iam-spoke-cluster
  namespace: default
spec:
  containers:
    - name: sleep
      image: mirror.gcr.io/busybox:1.36
      command: ["/bin/sh", "-c", "sleep infinity"]
EOF

echo "[4/9] Wait for ServiceAccount token Secret and read token"
# Wait for the controller to populate the token field in the Secret
for i in {1..30}; do
  DATA=$(kubectl --context "kind-spoke" get secret "spoke-reader-token" -o jsonpath='{.data.token}' 2>/dev/null || true)
  if [[ -n "${DATA}" ]]; then
    break
  fi
  sleep 1
done
if [[ -z "${DATA}" ]]; then
  echo "ERROR: failed to obtain token from Secret spoke-reader-token" >&2
  exit 1
fi
TOKEN=$(kubectl --context "kind-spoke" get secret "spoke-reader-token" -o go-template='{{.data.token | base64decode}}')

echo "[5/9] Extract spoke cluster server and CA data"
SERVER=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name==\"kind-spoke\")].cluster.server}")
CADATA=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name==\"kind-spoke\")].cluster.certificate-authority-data}")
if [[ -z "${SERVER}" || -z "${CADATA}" ]]; then
  echo "ERROR: failed to resolve spoke cluster server/CA data from kubeconfig" >&2
  exit 1
fi

echo "[6/9] Create hub cluster"
kind create cluster --name "hub"
kind get kubeconfig --name "hub" > "${EXAMPLE_DIR}/hub.kubeconfig"

echo "[7/9] Install ClusterProfile CRD on hub cluster"
kubectl --context "kind-hub" apply -f "${REPO_ROOT}/config/crd/bases/multicluster.x-k8s.io_clusterprofiles.yaml"

echo "[8/9] Create Secret on hub cluster with token"
kubectl --context "kind-hub" create secret generic "spoke-1" \
  --from-literal=token="${TOKEN}" \
  --dry-run=client -o yaml | kubectl --context "kind-hub" apply -f -

echo "[9/9] Create ClusterProfile on hub cluster and patch status with provider"
kubectl --context "kind-hub" apply -f - <<EOF
apiVersion: multicluster.x-k8s.io/v1alpha1
kind: ClusterProfile
metadata:
  name: spoke-1
  namespace: default
spec:
  clusterManager:
    name: demo
EOF

STATUS_PATCH=$(cat <<EOF
{
  "status": {
    "accessProviders": [
      {
        "name": "secretreader",
        "cluster": {
          "server": "${SERVER}",
          "certificate-authority-data": "${CADATA}",
          "extensions": [
            {
              "name": "client.authentication.k8s.io/exec",
              "extension": {
                "clusterName": "spoke-1"
              }
            }
          ]
        }
      }
    ]
  }
}
EOF
)

kubectl --context "kind-hub" patch clusterprofile "spoke-1" --type=merge --subresource=status -p "${STATUS_PATCH}"

