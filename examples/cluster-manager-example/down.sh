#!/bin/bash
set -euo pipefail

# Teardown script: removes the Kind hub cluster created by setup-kind.sh

echo "==> Deleting Kind cluster 'hub'..."
kind delete cluster --name hub || true

echo "=== Teardown complete ==="
