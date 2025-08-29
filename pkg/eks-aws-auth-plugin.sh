#!/usr/bin/env bash
set -euo pipefail

# -------- utils --------
err() { printf "[eks-exec-credential] %s\n" "$*" >&2; }
need() { command -v "$1" >/dev/null 2>&1 || { err "missing dependency: $1"; exit 1; }; }
normalize_host() { sed -E 's#^https?://##; s#/$##; s#:443$##'; }

need jq
need aws

# --- read ExecCredential ---
if [[ -z "${KUBERNETES_EXEC_INFO:-}" ]]; then
  err "KUBERNETES_EXEC_INFO is empty. set provideClusterInfo: true"
  exit 1
fi

REQ_API_VERSION="$(jq -r '.apiVersion // empty' <<<"$KUBERNETES_EXEC_INFO")"
SERVER="$(jq -r '.spec.cluster.server // empty' <<<"$KUBERNETES_EXEC_INFO")"
if [[ -z "$SERVER" || "$SERVER" == "null" ]]; then
  err "spec.cluster.server is missing in KUBERNETES_EXEC_INFO"
  exit 1
fi

NORM_SERVER="$(printf "%s" "$SERVER" | normalize_host)"

# --- region: infer from server hostname ---
HOST="${NORM_SERVER%%/*}"
REGION="$(printf "%s\n" "$HOST" \
  | sed -nE 's#.*\.([a-z0-9-]+)\.eks(-fips)?\.amazonaws\.com(\.cn)?$#\1#p')"
if [[ -z "$REGION" ]]; then
  err "failed to parse region from server hostname: ${SERVER}"
  err "expected something like ...<random>.<suffix>.<region>.eks.amazonaws.com"
  exit 1
fi

# --- tiny cache: endpoint -> cluster name ---
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/eks-exec-credential"
mkdir -p "$CACHE_DIR"
MAP_CACHE="$CACHE_DIR/endpoint-map-${REGION}.json"
if [[ ! -s "$MAP_CACHE" ]] || ! jq -e . >/dev/null 2>&1 <"$MAP_CACHE"; then
  echo '{}' >"$MAP_CACHE"
fi

lookup_cache() { jq -r --arg k "$NORM_SERVER" '.[$k] // empty' <"$MAP_CACHE"; }
update_cache() {
  local tmp; tmp="$(mktemp)"
  jq --arg k "$NORM_SERVER" --arg v "$1" '.[$k]=$v' "$MAP_CACHE" >"$tmp" && mv "$tmp" "$MAP_CACHE"
}

match_endpoint() {
  local name="$1"
  local ep norm_ep
  ep="$(aws eks describe-cluster --region "$REGION" --name "$name" \
        --query 'cluster.endpoint' --output text 2>/dev/null || true)"
  [[ -z "$ep" || "$ep" == "None" ]] && return 1
  norm_ep="$(printf "%s" "$ep" | normalize_host)"
  [[ "$norm_ep" == "$NORM_SERVER" ]]
}

CLUSTER_NAME=""
# 1) cache hit?
CACHED="$(lookup_cache || true)"
if [[ -n "$CACHED" ]] && match_endpoint "$CACHED"; then
  CLUSTER_NAME="$CACHED"
fi

# 2) enumerate if needed
if [[ -z "$CLUSTER_NAME" ]]; then
  err "resolving cluster in ${REGION} for ${NORM_SERVER}"
  found=""
  while IFS= read -r name; do
    [[ -z "$name" ]] && continue
    if match_endpoint "$name"; then
      found="$name"
      break
    fi
  done < <(aws eks list-clusters --region "$REGION" --output json | jq -r '.clusters[]?')

  if [[ -z "$found" ]]; then
    err "no matching EKS cluster for endpoint: ${SERVER} (region=${REGION})"
    exit 1
  fi
  CLUSTER_NAME="$found"
  update_cache "$CLUSTER_NAME" || true
fi

# --- fetch ExecCredential via aws CLI ---
TOKEN_JSON="$(aws eks get-token --region "$REGION" --cluster-name "$CLUSTER_NAME" --output json)"

printf "%s\n" "$TOKEN_JSON"
