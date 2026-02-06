#!/usr/bin/env bash
# Build and push a single plugin OCI image with buildx (binary at /bin/<name>-plugin).
# Usage: release-plugins.sh <plugin-name>
# Requires: REGISTRY (e.g. ghcr.io/kubernetes-sigs/cluster-inventory-api), VERSION, buildx.

set -o errexit
set -o nounset
set -o pipefail

name="$1"

docker buildx build \
	-f hack/Dockerfile.plugin \
	--build-arg "PLUGIN_NAME=${name}" \
	--platform=linux/amd64,linux/arm64 \
	-t "${REGISTRY}/${name}:${VERSION}" \
	--push \
	--attest type=provenance,mode=max \
	--attest type=sbom \
	.
