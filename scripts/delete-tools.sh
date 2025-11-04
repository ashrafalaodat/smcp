#!/usr/bin/env bash

set -euo pipefail

# Deletes tools from the registry. Provide tool IDs as arguments or via the
# TOOL_IDS environment variable (space-separated). You can also supply OWNER
# (and optional NAME/QUERY) to delete every matching tool.
#
# Environment overrides:
#   BASE_URL       - Registry base URL. If unset and PORT_FORWARD=true, a port-forward
#                    will be started automatically.
#   PORT_FORWARD   - When "true", port-forward svc/SERVICE_NAME before deleting (default: false)
#   NAMESPACE      - Kubernetes namespace for port-forwarding (default: default)
#   SERVICE_NAME   - Kubernetes service name used for port-forwarding (default: mcp-registry)
#   FORWARD_PORT   - Local port to bind during port-forward (default: 18080)
#   REMOTE_PORT    - Target service port for port-forward (default: 80)
#   TOOL_IDS       - Optional space-separated list of tool IDs to delete
#   OWNER          - Optional owner filter when TOOL_IDS is empty
#   NAME           - Optional name filter when using OWNER-based deletion
#   QUERY          - Optional text query filter when using OWNER-based deletion
#   DRY_RUN        - When "true", print the targets without deleting (default: false)

PORT_FORWARD="${PORT_FORWARD:-false}"
NAMESPACE="${NAMESPACE:-default}"
SERVICE_NAME="${SERVICE_NAME:-mcp-registry}"
FORWARD_PORT="${FORWARD_PORT:-18080}"
REMOTE_PORT="${REMOTE_PORT:-80}"

BASE_URL="${BASE_URL:-}"
OWNER="${OWNER:-}"
NAME="${NAME:-}"
QUERY="${QUERY:-}"
DRY_RUN="${DRY_RUN:-false}"

PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""

cleanup() {
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    kill "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${PORT_FORWARD_LOG}" && -f "${PORT_FORWARD_LOG}" ]]; then
    rm -f "${PORT_FORWARD_LOG}"
  fi
}

trap cleanup EXIT

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

start_port_forward() {
  if [[ "${PORT_FORWARD}" != "true" ]]; then
    return
  fi
  if ! command -v kubectl >/dev/null 2>&1; then
    echo "kubectl is required for port-forwarding" >&2
    exit 1
  fi
  PORT_FORWARD_LOG="$(mktemp)"
  echo "Starting port-forward to svc/${SERVICE_NAME} in namespace ${NAMESPACE}..."
  kubectl -n "${NAMESPACE}" port-forward "svc/${SERVICE_NAME}" "${FORWARD_PORT}:${REMOTE_PORT}" >"${PORT_FORWARD_LOG}" 2>&1 &
  PORT_FORWARD_PID=$!
  BASE_URL="http://127.0.0.1:${FORWARD_PORT}"
  for attempt in $(seq 1 20); do
    if curl -s -o /dev/null "${BASE_URL}/healthz" || curl -s -o /dev/null "${BASE_URL}/"; then
      return
    fi
    sleep 1
    if ! kill -0 "${PORT_FORWARD_PID}" >/dev/null 2>&1; then
      echo "Port-forward exited unexpectedly. See log at ${PORT_FORWARD_LOG}" >&2
      exit 1
    fi
  done
  echo "Timed out waiting for port-forward to become ready. See log at ${PORT_FORWARD_LOG}" >&2
  exit 1
}

read_ids() {
  local list=()
  if [[ $# -gt 0 ]]; then
    list+=("$@")
  fi
  if [[ -n "${TOOL_IDS:-}" ]]; then
    for id in ${TOOL_IDS}; do
      list+=("$id")
    done
  fi
  echo "${list[@]}"
}

fetch_ids_by_filter() {
  if [[ -z "${OWNER}" ]]; then
    echo ""
    return
  fi
  if ! command -v jq >/dev/null 2>&1; then
    echo "jq is required to resolve IDs by owner" >&2
    exit 1
  fi
  local query=()
  query+=("owner=$(printf '%s' "${OWNER}" | jq -s -R -r @uri)")
  [[ -n "${NAME}" ]] && query+=("name=$(printf '%s' "${NAME}" | jq -s -R -r @uri)")
  [[ -n "${QUERY}" ]] && query+=("q=$(printf '%s' "${QUERY}" | jq -s -R -r @uri)")
  local qs=""
  if [[ ${#query[@]} -gt 0 ]]; then
    qs="?"$(IFS='&'; echo "${query[*]}")
  fi
  local response status body
  response=$(curl -sS -w "\n%{http_code}" -X GET "${BASE_URL}/v1/tools${qs}")
  status=${response##*$'\n'}
  body=${response%$'\n'$status}
  if [[ "${status}" != "200" ]]; then
    echo "Failed to list tools for deletion (status=${status}): ${body}" >&2
    exit 1
  fi
  echo "${body}" | jq -r '.[].id'
}

start_port_forward

if [[ -z "${BASE_URL}" ]]; then
  BASE_URL="http://127.0.0.1:8080"
fi

ids=($(read_ids "$@"))
if [[ ${#ids[@]} -eq 0 ]]; then
  mapfile -t ids < <(fetch_ids_by_filter)
fi

if [[ ${#ids[@]} -eq 0 ]]; then
  echo "No tool IDs supplied. Provide arguments, set TOOL_IDS, or specify OWNER/NAME/QUERY." >&2
  exit 1
fi

echo "Preparing to delete ${#ids[@]} tool(s)."
if [[ "${DRY_RUN}" == "true" ]]; then
  printf 'DRY RUN - would delete IDs:\n'
  printf '  %s\n' "${ids[@]}"
  exit 0
fi

failed=0
for id in "${ids[@]}"; do
  response=$(curl -sS -w "\n%{http_code}" -X DELETE "${BASE_URL}/v1/tools/${id}")
  status=${response##*$'\n'}
  body=${response%$'\n'$status}
  if [[ "${status}" == "204" || "${status}" == "202" || "${status}" == "200" ]]; then
    echo "✔ Deleted ${id}"
  elif [[ "${status}" == "404" ]]; then
    echo "⚠ Tool ${id} not found" >&2
  else
    echo "✖ Failed to delete ${id} (status=${status}): ${body}" >&2
    failed=$((failed + 1))
  fi
done

if [[ ${failed} -gt 0 ]]; then
  echo "${failed} tool(s) failed to delete." >&2
  exit 1
fi

echo "Deletion complete."
