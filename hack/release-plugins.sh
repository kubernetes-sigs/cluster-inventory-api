#!/usr/bin/env bash
# Build and push a single plugin OCI image with ko.
# Usage: release-plugins.sh <plugin-name>

set -o errexit
set -o nounset
set -o pipefail

name="$1"
version=$(tr -d '[:space:]' < "plugins/${name}/VERSION")

KO_DOCKER_REPO="${REGISTRY}/${name}" ko build "./plugins/${name}/cmd/plugin" \
  --bare --tags "${version}" --platform=linux/amd64,linux/arm64 --push
