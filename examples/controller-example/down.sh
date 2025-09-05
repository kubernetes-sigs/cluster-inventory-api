#!/usr/bin/env bash
set -euo pipefail

kind delete cluster --name "hub" || true
kind delete cluster --name "spoke" || true
