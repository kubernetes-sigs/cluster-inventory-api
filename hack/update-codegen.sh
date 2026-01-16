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

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CODEGEN_PKG="${CODEGEN_PKG:-}"

if [[ -z "${CODEGEN_PKG}" ]]; then
    # Resolve from Go module cache
    moddir="$(cd "${SCRIPT_ROOT}" && go list -m -f '{{.Dir}}' k8s.io/code-generator 2>/dev/null || true)"
    CODEGEN_PKG="${moddir}"
fi

if [[ ! -f "${CODEGEN_PKG}/kube_codegen.sh" ]]; then
    echo "error: kube_codegen.sh not found at ${CODEGEN_PKG}" >&2
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
