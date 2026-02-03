#!/bin/bash

# Copyright 2024 The Kubernetes Authors.
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
go mod download k8s.io/code-generator

# Find code-generator from go modules
CODEGEN_PKG=${CODEGEN_PKG:-$(go list -m -f '{{.Dir}}' k8s.io/code-generator)}

if [[ -z "${CODEGEN_PKG}" ]]; then
  echo "ERROR: Could not find k8s.io/code-generator module"
  exit 1
fi

source "${CODEGEN_PKG}/kube_codegen.sh"

THIS_PKG="sigs.k8s.io/cluster-inventory-api"

kube::codegen::gen_helpers \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}"

kube::codegen::gen_client \
    --with-watch \
    --output-dir "${SCRIPT_ROOT}/client" \
    --output-pkg "${THIS_PKG}/client" \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    --one-input-api apis \
    "${SCRIPT_ROOT}"
