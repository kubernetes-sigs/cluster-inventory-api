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

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE}")/..

cd $SCRIPT_ROOT

# Create temp directories for diffing
DIFFROOT_APIS="${SCRIPT_ROOT}/apis"
DIFFROOT_CLIENT="${SCRIPT_ROOT}/client"
TMP_DIFFROOT_APIS="${SCRIPT_ROOT}/_tmp/apis"
TMP_DIFFROOT_CLIENT="${SCRIPT_ROOT}/_tmp/client"
_tmp="${SCRIPT_ROOT}/_tmp"

cleanup() {
  rm -rf "${_tmp}"
}
trap "cleanup" EXIT SIGINT

cleanup

mkdir -p "${TMP_DIFFROOT_APIS}"
mkdir -p "${TMP_DIFFROOT_CLIENT}"
cp -a "${DIFFROOT_APIS}"/* "${TMP_DIFFROOT_APIS}"
cp -a "${DIFFROOT_CLIENT}"/* "${TMP_DIFFROOT_CLIENT}"

bash "${SCRIPT_ROOT}/hack/update-codegen.sh"

echo "diffing ${DIFFROOT_APIS} against freshly generated codegen"
ret=0
diff -Naupr "${DIFFROOT_APIS}" "${TMP_DIFFROOT_APIS}" || ret=$?
if [[ $ret -ne 0 ]]; then
  cp -a "${TMP_DIFFROOT_APIS}"/* "${DIFFROOT_APIS}"
  echo "${DIFFROOT_APIS} is out of date. Please run hack/update-codegen.sh"
  exit 1
fi
echo "${DIFFROOT_APIS} up to date."

echo "diffing ${DIFFROOT_CLIENT} against freshly generated codegen"
diff -Naupr "${DIFFROOT_CLIENT}" "${TMP_DIFFROOT_CLIENT}" || ret=$?
if [[ $ret -ne 0 ]]; then
  cp -a "${TMP_DIFFROOT_CLIENT}"/* "${DIFFROOT_CLIENT}"
  echo "${DIFFROOT_CLIENT} is out of date. Please run hack/update-codegen.sh"
  exit 1
fi
echo "${DIFFROOT_CLIENT} up to date."

cp -a "${TMP_DIFFROOT_APIS}"/* "${DIFFROOT_APIS}"
cp -a "${TMP_DIFFROOT_CLIENT}"/* "${DIFFROOT_CLIENT}"
