#!/bin/bash

# Copyright 2025 The Kubernetes Authors.
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

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

cd "${SCRIPT_ROOT}"

# Install code-generator tools
echo "Installing code-generator tools..."
go install k8s.io/code-generator/cmd/client-gen@v0.32.1
go install k8s.io/code-generator/cmd/lister-gen@v0.32.1
go install k8s.io/code-generator/cmd/informer-gen@v0.32.1

# Go installs the above commands to $GOBIN if defined, and $GOPATH/bin otherwise
GOBIN="$(go env GOBIN)"
gobin="${GOBIN:-$(go env GOPATH)/bin}"

THIS_PKG="sigs.k8s.io/cluster-inventory-api"
OUTPUT_PKG="${THIS_PKG}/client"
FQ_APIS="${THIS_PKG}/apis/v1alpha1"

echo "Generating clientset at ${OUTPUT_PKG}/clientset"
"${gobin}/client-gen" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --input-base="" \
  --input="${FQ_APIS}" \
  --output-pkg="${OUTPUT_PKG}/clientset" \
  --output-dir="client/clientset" \
  --clientset-name=versioned

echo "Generating listers at ${OUTPUT_PKG}/listers"
"${gobin}/lister-gen" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-pkg="${OUTPUT_PKG}/listers" \
  --output-dir="client/listers" \
  "${FQ_APIS}"

echo "Generating informers at ${OUTPUT_PKG}/informers"
"${gobin}/informer-gen" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --versioned-clientset-package="${OUTPUT_PKG}/clientset/versioned" \
  --listers-package="${OUTPUT_PKG}/listers" \
  --output-pkg="${OUTPUT_PKG}/informers" \
  --output-dir="client/informers" \
  "${FQ_APIS}"

echo "Code generation completed."
