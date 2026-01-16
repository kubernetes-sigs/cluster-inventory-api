#!/bin/bash
set -euo pipefail

# This script sets up a demo with:
# - kind hub cluster and kind spoke cluster
# - extract kubeconfig from spoke cluster (server, CA, client cert/key)
# - a Secret on the hub cluster with kubeconfig YAML
# - the ClusterProfile CR on the hub cluster with kubeconfig-secretreader provider pointing to the spoke cluster

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
EXAMPLE_DIR=$(cd "${SCRIPT_DIR}/../.." && pwd)
REPO_ROOT=$(cd "${EXAMPLE_DIR}/../.." && pwd)

echo "[1/7] Create spoke cluster"
kind create cluster --name "spoke"
# Wait for default ServiceAccount to be created in the default namespace on the spoke cluster
for i in {1..60}; do
  if kubectl --context "kind-spoke" -n default get sa default >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "[2/7] Create a sleep Pod on spoke cluster"
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

echo "[3/7] Extract spoke cluster kubeconfig data"
SERVER=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name==\"kind-spoke\")].cluster.server}")
CADATA=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name==\"kind-spoke\")].cluster.certificate-authority-data}")
CONTEXT_NAME=$(kubectl config view --raw -o jsonpath="{.contexts[?(@.name==\"kind-spoke\")].name}")
USER_NAME=$(kubectl config view --raw -o jsonpath="{.contexts[?(@.name==\"kind-spoke\")].context.user}")
CLIENT_CERT_DATA=$(kubectl config view --raw -o jsonpath="{.users[?(@.name==\"${USER_NAME}\")].user.client-certificate-data}")
CLIENT_KEY_DATA=$(kubectl config view --raw -o jsonpath="{.users[?(@.name==\"${USER_NAME}\")].user.client-key-data}")

if [[ -z "${SERVER}" || -z "${CADATA}" || -z "${CONTEXT_NAME}" || -z "${USER_NAME}" ]]; then
  echo "ERROR: failed to resolve spoke cluster kubeconfig data" >&2
  exit 1
fi

if [[ -z "${CLIENT_CERT_DATA}" || -z "${CLIENT_KEY_DATA}" ]]; then
  echo "ERROR: failed to resolve client certificate/key data from kubeconfig" >&2
  exit 1
fi

echo "[4/7] Create hub cluster"
kind create cluster --name "hub"
kind get kubeconfig --name "hub" > "${EXAMPLE_DIR}/hub.kubeconfig"

echo "[5/7] Install ClusterProfile CRD on hub cluster"
kubectl --context "kind-hub" apply -f "${REPO_ROOT}/config/crd/bases/multicluster.x-k8s.io_clusterprofiles.yaml"

echo "[6/7] Create Secret on hub cluster with kubeconfig"
# Build minimal kubeconfig YAML
KUBECONFIG_YAML=$(cat <<EOF
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: ${CADATA}
    server: ${SERVER}
  name: spoke-cluster
contexts:
- context:
    cluster: spoke-cluster
    user: ${USER_NAME}
  name: ${CONTEXT_NAME}
current-context: ${CONTEXT_NAME}
kind: Config
users:
- name: ${USER_NAME}
  user:
    client-certificate-data: ${CLIENT_CERT_DATA}
    client-key-data: ${CLIENT_KEY_DATA}
EOF
)

kubectl --context "kind-hub" create secret generic "spoke-1-kubeconfig" \
  --from-literal=value="${KUBECONFIG_YAML}" \
  --dry-run=client -o yaml | kubectl --context "kind-hub" apply -f -

echo "[7/7] Create ClusterProfile on hub cluster and patch status with provider"
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
        "name": "kubeconfig-secretreader",
        "cluster": {
          "server": "${SERVER}",
          "certificate-authority-data": "${CADATA}",
          "extensions": [
            {
              "name": "client.authentication.k8s.io/exec",
              "extension": {
                "name": "spoke-1-kubeconfig",
                "key": "value",
                "namespace": "default",
                "context": "${CONTEXT_NAME}"
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

