#!/usr/bin/env bash

set -euo pipefail

# Retrieves tools from the registry. You may fetch a specific tool by ID or
# list tools filtered by owner/name/query.
#
# Environment overrides:
#   BASE_URL       - Registry base URL. If unset and PORT_FORWARD=true, a port-forward
#                    will be started automatically.
#   PORT_FORWARD   - When "true", port-forward svc/SERVICE_NAME before querying (default: false)
#   NAMESPACE      - Kubernetes namespace for port-forwarding (default: default)
#   SERVICE_NAME   - Kubernetes service name used for port-forwarding (default: mcp-registry)
#   FORWARD_PORT   - Local port to bind during port-forward (default: 18080)
#   REMOTE_PORT    - Target service port for port-forward (default: 80)
#   TOOL_ID        - Optional tool UUID to fetch (takes precedence over list parameters)
#   OWNER          - Optional owner filter for list queries
#   NAME           - Optional name filter for list queries
#   QUERY          - Optional text query filter (applies to name/description)
#   RAW_OUTPUT     - When "true", print JSON without pretty formatting (default: false)

PORT_FORWARD="${PORT_FORWARD:-false}"
NAMESPACE="${NAMESPACE:-default}"
SERVICE_NAME="${SERVICE_NAME:-mcp-registry}"
FORWARD_PORT="${FORWARD_PORT:-18080}"
REMOTE_PORT="${REMOTE_PORT:-80}"

BASE_URL="${BASE_URL:-}"
TOOL_ID="${TOOL_ID:-}"
OWNER="${OWNER:-}"
NAME="${NAME:-}"
QUERY="${QUERY:-}"
RAW_OUTPUT="${RAW_OUTPUT:-false}"

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

pretty_print() {
  if [[ "${RAW_OUTPUT}" == "true" ]] || ! command -v jq >/dev/null 2>&1; then
    cat
  else
    jq .
  fi
}

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

start_port_forward

if [[ -z "${BASE_URL}" ]]; then
  BASE_URL="http://127.0.0.1:8080"
fi

if [[ -n "${TOOL_ID}" ]]; then
  path="/v1/tools/${TOOL_ID}"
else
  query=()
  [[ -n "${OWNER}" ]] && query+=("owner=$(printf '%s' "${OWNER}" | jq -s -R -r @uri)")
  [[ -n "${NAME}" ]] && query+=("name=$(printf '%s' "${NAME}" | jq -s -R -r @uri)")
  [[ -n "${QUERY}" ]] && query+=("q=$(printf '%s' "${QUERY}" | jq -s -R -r @uri)")
  qs=""
  if [[ ${#query[@]} -gt 0 ]]; then
    qs="?"$(IFS='&'; echo "${query[*]}")
  fi
  path="/v1/tools${qs}"
fi

response=$(curl -sS -w "\n%{http_code}" -X GET "${BASE_URL}${path}")
status=${response##*$'\n'}
body=${response%$'\n'$status}

if [[ "${status}" != "200" ]]; then
  echo "Request failed (status=${status}):" >&2
  echo "${body}" >&2
  exit 1
fi

pretty_print <<<"${body}"
