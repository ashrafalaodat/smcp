#!/usr/bin/env bash

set -euo pipefail

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required to run this script." >&2
  exit 1
fi

NAMESPACE=${NAMESPACE:-default}
SERVICE_NAME=${SERVICE_NAME:-mcp-registry}
FORWARD_PORT=${FORWARD_PORT:-18080}
DISABLE_VECTORIZE=${DISABLE_VECTORIZE:-true}
TMP_DIR=$(mktemp -d)
PF_PID=""

cleanup() {
  if [[ -n "${PF_PID}" ]] && kill -0 "${PF_PID}" 2>/dev/null; then
    kill "${PF_PID}" >/dev/null 2>&1 || true
    wait "${PF_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if [[ "${DISABLE_VECTORIZE}" == "true" && -z "${BASE_URL:-}" ]]; then
  if command -v kubectl >/dev/null 2>&1; then
    if kubectl -n "${NAMESPACE}" get deployment "${SERVICE_NAME}" >/dev/null 2>&1; then
      echo ">>> Ensuring vectorizer disabled on deployment/${SERVICE_NAME}"
      kubectl -n "${NAMESPACE}" set env "deployment/${SERVICE_NAME}" MCP_REGISTRY_VECTORIZE_URL= --overwrite >/dev/null
      kubectl -n "${NAMESPACE}" rollout status "deployment/${SERVICE_NAME}" --timeout=180s >/dev/null
    fi
  fi
fi

if [[ -z "${BASE_URL:-}" ]]; then
  if command -v kubectl >/dev/null 2>&1; then
    PORT_FORWARD_LOG="${TMP_DIR}/port-forward.log"
    kubectl -n "${NAMESPACE}" port-forward "svc/${SERVICE_NAME}" "${FORWARD_PORT}:80" >"${PORT_FORWARD_LOG}" 2>&1 &
    PF_PID=$!
    BASE_URL="http://127.0.0.1:${FORWARD_PORT}"

    for attempt in {1..30}; do
      if curl -sSf "${BASE_URL}/v1/tools" >/dev/null 2>&1; then
        break
      fi
      if ! kill -0 "${PF_PID}" 2>/dev/null; then
        echo "port-forward process exited unexpectedly" >&2
        cat "${PORT_FORWARD_LOG}" >&2 || true
        exit 1
      fi
      sleep 1
      if [[ "${attempt}" -eq 30 ]]; then
        echo "timed out waiting for port-forward to ${SERVICE_NAME}" >&2
        cat "${PORT_FORWARD_LOG}" >&2 || true
        exit 1
      fi
    done
  else
    BASE_URL="http://localhost:8080"
  fi
fi

SUFFIX=$(python3 - <<'PY'
import uuid
print(uuid.uuid4().hex[:8])
PY
)
TOOL_OWNER="team-search"
TOOL_NAME="vector-summarize-${SUFFIX}"
POLICY_PRINCIPAL="service:agent"
AUDIT_ACTOR="agent/42"

CREATE_PAYLOAD=$(cat <<EOF
{
  "owner": "${TOOL_OWNER}",
  "name": "${TOOL_NAME}",
  "description": "Summarises documents into short bullet lists",
  "inputs": {"text": "string"},
  "outputs": {"summary": "string"},
  "policies": [{
    "principal": "${POLICY_PRINCIPAL}",
    "allowed_scope": ["summaries"],
    "conditions": {"region": "us-east"}
  }]
}
EOF
)

echo ">>> POST /v1/tools"
create_response=$(curl --fail-with-body -sS -X POST "${BASE_URL}/v1/tools" \
  -H 'Content-Type: application/json' \
  -d "${CREATE_PAYLOAD}")
echo "${create_response}"

TOOL_ID=$(CREATE_RESPONSE="${create_response}" EXPECTED_OWNER="${TOOL_OWNER}" EXPECTED_NAME="${TOOL_NAME}" EXPECTED_PRINCIPAL="${POLICY_PRINCIPAL}" python3 - <<'PY'
import json
import os
import sys

data = json.loads(os.environ["CREATE_RESPONSE"])
tool = data["tool"]
policies = data["policies"]
assert tool["owner"] == os.environ["EXPECTED_OWNER"], f"owner mismatch: {tool['owner']}"
assert tool["name"] == os.environ["EXPECTED_NAME"], f"name mismatch: {tool['name']}"
assert len(policies) == 1, f"expected 1 policy, got {len(policies)}"
assert policies[0]["principal"] == os.environ["EXPECTED_PRINCIPAL"], "policy principal mismatch"
print(tool["id"])
PY
)

echo -e "\n\n>>> GET /v1/tools (filtered)"
list_response=$(curl --fail-with-body -sS -G "${BASE_URL}/v1/tools" \
  --data-urlencode "owner=${TOOL_OWNER}" \
  --data-urlencode "name=${TOOL_NAME}")
echo "${list_response}"

python3 - TOOL_ID="${TOOL_ID}" RESPONSE="${list_response}" <<'PY'
import json, os, sys
tools = json.loads(os.environ["RESPONSE"])
target = os.environ["TOOL_ID"]
if not any(t.get("id") == target for t in tools):
    sys.exit(f"tool id {target} missing from /v1/tools response")
PY

echo -e "\n\n>>> GET /v1/tools/${TOOL_ID}"
get_response=$(curl --fail-with-body -sS "${BASE_URL}/v1/tools/${TOOL_ID}")
echo "${get_response}"
python3 - TOOL_ID="${TOOL_ID}" NAME="${TOOL_NAME}" PRINCIPAL="${POLICY_PRINCIPAL}" RESPONSE="${get_response}" <<'PY'
import json, os, sys
resp = json.loads(os.environ["RESPONSE"])
tool = resp["tool"]
policies = resp["policies"]
audits = resp["audits"]
assert tool["id"] == os.environ["TOOL_ID"], "tool id mismatch"
assert tool["name"] == os.environ["NAME"], "tool name mismatch"
assert len(policies) == 1 and policies[0]["principal"] == os.environ["PRINCIPAL"], "policy mismatch"
assert len(audits) == 0, f"expected no audits yet, got {len(audits)}"
PY

echo -e "\n\n>>> POST /v1/tools/${TOOL_ID}/audits"
audit_response=$(curl --fail-with-body -sS -X POST "${BASE_URL}/v1/tools/${TOOL_ID}/audits" \
  -H 'Content-Type: application/json' \
  -d '{
    "event_type": "invocation",
    "actor": "'"${AUDIT_ACTOR}"'",
    "payload": {"status": "success"}
  }')
echo "${audit_response}"

python3 - RESPONSE="${audit_response}" EXPECTED_ACTOR="${AUDIT_ACTOR}" <<'PY'
import json, os
data = json.loads(os.environ["RESPONSE"])
assert data["actor"] == os.environ["EXPECTED_ACTOR"], "audit actor mismatch"
assert data["event_type"] == "invocation", "audit event type mismatch"
PY

echo -e "\n\n>>> GET /v1/tools/${TOOL_ID} (after audit)"
final_response=$(curl --fail-with-body -sS "${BASE_URL}/v1/tools/${TOOL_ID}")
echo "${final_response}"
python3 - RESPONSE="${final_response}" EXPECTED_ACTOR="${AUDIT_ACTOR}" <<'PY'
import json, os, sys
resp = json.loads(os.environ["RESPONSE"])
audits = resp.get("audits", [])
assert audits, "expected at least one audit entry"
assert audits[0]["actor"] == os.environ["EXPECTED_ACTOR"], "audit actor mismatch after retrieval"
assert audits[0]["event_type"] == "invocation", "audit event type mismatch after retrieval"
PY

echo -e "\nE2E test PASSED"
