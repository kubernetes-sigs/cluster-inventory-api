#!/bin/bash

# Copyright The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# E2E: Run controller-example as a Job in the hub cluster with plugin OCI
# image mounted via image volume. Assumes hub and spoke kind clusters already
# exist and setup-kind-demo.sh was run.
# Usage: e2e-controller-incluster.sh <plugin_name> <provider_name>
# Example: e2e-controller-incluster.sh secretreader secretreader

set -o errexit
set -o nounset
set -o pipefail

PLUGIN_NAME="${1:?usage: e2e-controller-incluster.sh <plugin_name> <provider_name>}"
PROVIDER_NAME="${2:?usage: e2e-controller-incluster.sh <plugin_name> <provider_name>}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

PLUGIN_IMAGE="localhost/${PLUGIN_NAME}:e2e"
CONTROLLER_IMAGE="localhost/controller-example:e2e"
DEPLOY_NAME="controller-example"
DEPLOY_NS="spoke-manager"
PROFILE_NS="fleet"

echo "--- Build plugin OCI image and load into hub"
docker buildx build \
	-f hack/Dockerfile.plugin \
	--build-arg "PLUGIN_NAME=${PLUGIN_NAME}" \
	-t "${PLUGIN_IMAGE}" \
	--load \
	.
kind load docker-image "${PLUGIN_IMAGE}" --name hub

echo "--- Build controller-example OCI image and load into hub"
docker buildx build \
	-f hack/Dockerfile.controller-example \
	-t "${CONTROLLER_IMAGE}" \
	--load \
	.
kind load docker-image "${CONTROLLER_IMAGE}" --name hub

echo "--- Patch ClusterProfile spoke-1 so spoke server is reachable from hub (spoke-control-plane:6443)"
kubectl --context kind-hub patch clusterprofile spoke-1 -n "${PROFILE_NS}" --type=json --subresource=status \
	-p '[{"op":"replace","path":"/status/accessProviders/0/cluster/server","value":"https://spoke-control-plane:6443"}]'

echo "--- Apply RBAC for controller-example on hub"
kubectl --context kind-hub apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: controller-example
  namespace: ${DEPLOY_NS}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: controller-example
  namespace: ${DEPLOY_NS}
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: controller-example
  namespace: ${DEPLOY_NS}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: controller-example
subjects:
  - kind: ServiceAccount
    name: controller-example
    namespace: ${DEPLOY_NS}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: controller-example
  namespace: ${PROFILE_NS}
rules:
  - apiGroups: ["multicluster.x-k8s.io"]
    resources: ["clusterprofiles"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: controller-example
  namespace: ${PROFILE_NS}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: controller-example
subjects:
  - kind: ServiceAccount
    name: controller-example
    namespace: ${DEPLOY_NS}
EOF

echo "--- Create provider-config ConfigMap (command stays ./bin/<name>-plugin; workingDir=/plugin)"
kubectl --context kind-hub create configmap controller-example-provider-config \
	--namespace "${DEPLOY_NS}" \
	--from-file=provider-config.json="examples/controller-example/plugins/${PLUGIN_NAME}/provider-config.json" \
	--dry-run=client -o yaml | kubectl --context kind-hub apply -f -

echo "--- Resolve spoke-control-plane IP for hostAliases"
SPOKE_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' spoke-control-plane)
if [[ -z "${SPOKE_IP}" ]]; then
	echo "ERROR: could not get spoke-control-plane container IP" >&2
	exit 1
fi

echo "--- Create controller-example Job in hub"
kubectl --context kind-hub apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${DEPLOY_NAME}
  namespace: ${DEPLOY_NS}
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: controller-example
    spec:
      restartPolicy: Never
      serviceAccountName: controller-example
      hostAliases:
        - hostnames: ["spoke-control-plane"]
          ip: "${SPOKE_IP}"
      containers:
        - name: controller
          image: ${CONTROLLER_IMAGE}
          imagePullPolicy: Never
          workingDir: /plugin
          args:
            - -clusterprofile-provider-file=/config/provider-config.json
            - -namespace=${PROFILE_NS}
            - -clusterprofile=spoke-1
          volumeMounts:
            - name: plugin-volume
              mountPath: /plugin
              readOnly: true
            - name: provider-config
              mountPath: /config
              readOnly: true
      volumes:
        - name: plugin-volume
          image:
            reference: ${PLUGIN_IMAGE}
            pullPolicy: Never
        - name: provider-config
          configMap:
            name: controller-example-provider-config
            items:
              - key: provider-config.json
                path: provider-config.json
EOF

echo "--- Wait for Job to finish"
STATE=""
for i in $(seq 1 60); do
	COMPLETE=$(kubectl --context kind-hub -n "${DEPLOY_NS}" get "job/${DEPLOY_NAME}" \
		-o jsonpath='{.status.conditions[?(@.type=="Complete")].status}' 2>/dev/null || true)
	if [[ "${COMPLETE}" == "True" ]]; then
		STATE="complete"
		break
	fi
	FAILED=$(kubectl --context kind-hub -n "${DEPLOY_NS}" get "job/${DEPLOY_NAME}" \
		-o jsonpath='{.status.conditions[?(@.type=="Failed")].status}' 2>/dev/null || true)
	if [[ "${FAILED}" == "True" ]]; then
		STATE="failed"
		break
	fi
	sleep 2
done

POD=$(kubectl --context kind-hub -n "${DEPLOY_NS}" get pods \
	-l "batch.kubernetes.io/job-name=${DEPLOY_NAME}" \
	-o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
LOGS=""
if [[ -n "${POD}" ]]; then
	LOGS=$(kubectl --context kind-hub -n "${DEPLOY_NS}" logs "${POD}" 2>&1 || true)
	echo "${LOGS}"
fi

case "${STATE}" in
	complete) ;;
	failed)
		echo "ERROR: Job failed (plugin: ${PLUGIN_NAME})" >&2
		kubectl --context kind-hub -n "${DEPLOY_NS}" describe "job/${DEPLOY_NAME}" >&2 || true
		exit 1
		;;
	*)
		echo "ERROR: Job timed out (plugin: ${PLUGIN_NAME})" >&2
		kubectl --context kind-hub -n "${DEPLOY_NS}" describe "job/${DEPLOY_NAME}" >&2 || true
		exit 1
		;;
esac

if [[ -z "${POD}" ]]; then
	echo "ERROR: no pod found for job/${DEPLOY_NAME}" >&2
	kubectl --context kind-hub get pods -A >&2 || true
	exit 1
fi

if ! echo "${LOGS}" | grep -q "\[client-go\] Listed"; then
	echo "ERROR: logs missing [client-go] Listed" >&2
	exit 1
fi
if ! echo "${LOGS}" | grep -q "\[controller-runtime\] Listed"; then
	echo "ERROR: logs missing [controller-runtime] Listed" >&2
	exit 1
fi

echo "--- Controller example in-cluster e2e passed (plugin: ${PLUGIN_NAME})"
