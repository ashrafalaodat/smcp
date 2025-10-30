#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${CONTAINER_NAME:-knative-localhost-proxy}"
CADDY_IMAGE="${CADDY_IMAGE:-caddy:2}"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required but not found in PATH" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required but not found in PATH" >&2
  exit 1
fi

# Allow overrides via environment but default to the first node/HTTP NodePort.
NODE_IP="${KOURIER_NODE_IP:-$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')}"
if [[ -z "${NODE_IP}" ]]; then
  echo "Unable to resolve the Knative node InternalIP. Set KOURIER_NODE_IP and retry." >&2
  exit 1
fi

NODE_PORT="${KOURIER_NODE_PORT:-$(kubectl get svc kourier -n knative-serving -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}')}"
if [[ -z "${NODE_PORT}" ]]; then
  echo "Unable to resolve the Kourier NodePort. Set KOURIER_NODE_PORT and retry." >&2
  exit 1
fi

UPSTREAM="${NODE_IP}:${NODE_PORT}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CADDYFILE="${SCRIPT_DIR}/Caddyfile"

if [[ ! -f "${CADDYFILE}" ]]; then
  echo "Caddyfile not found at ${CADDYFILE}" >&2
  exit 1
fi

echo "Starting ${CONTAINER_NAME} -> ${UPSTREAM}"
docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true

docker run -d \
  --name "${CONTAINER_NAME}" \
  --network host \
  -e "KOURIER_UPSTREAM=${UPSTREAM}" \
  -v "${CADDYFILE}:/etc/caddy/Caddyfile:ro" \
  "${CADDY_IMAGE}" >/dev/null

echo "Proxy ready: http://{function}.{namespace}.localhost"
