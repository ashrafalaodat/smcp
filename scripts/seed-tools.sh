#!/usr/bin/env bash

set -euo pipefail

# Seeds the registry with a batch of tools. It creates 20 tools under the same
# owner namespace, with the first five descriptions intentionally similar so
# semantic search can surface them together.
#
# Environment overrides:
#   BASE_URL       - Registry base URL. If unset and PORT_FORWARD=true, a port-forward
#                    will be started automatically to construct this value.
#   PORT_FORWARD   - When "true", port-forward svc/SERVICE_NAME before seeding (default: false)
#   NAMESPACE      - Kubernetes namespace for port-forwarding (default: default)
#   SERVICE_NAME   - Kubernetes service name used for port-forwarding (default: mcp-registry)
#   FORWARD_PORT   - Local port to bind during port-forward (default: 18080)
#   REMOTE_PORT    - Target service port for port-forward (default: 8080)
#   TOOL_OWNER     - Owner/namespace for the seeded tools (default: demo-namespace)
#   TOOL_PREFIX    - Name prefix for the seeded tools (default: demo-tool)

PORT_FORWARD="${PORT_FORWARD:-false}"
NAMESPACE="${NAMESPACE:-default}"
SERVICE_NAME="${SERVICE_NAME:-mcp-registry}"
FORWARD_PORT="${FORWARD_PORT:-18080}"
REMOTE_PORT="${REMOTE_PORT:-8080}"

BASE_URL="${BASE_URL:-}"
TOOL_OWNER="${TOOL_OWNER:-demo-namespace}"
TOOL_PREFIX="${TOOL_PREFIX:-demo-tool}"
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

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

if [[ "${PORT_FORWARD}" == "true" ]]; then
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
    if curl -s -o /dev/null "${BASE_URL}/"; then
      break
    fi
    sleep 1
    if ! kill -0 "${PORT_FORWARD_PID}" >/dev/null 2>&1; then
      echo "Port-forward exited unexpectedly. See log at ${PORT_FORWARD_LOG}" >&2
      exit 1
    fi
    if [[ "${attempt}" -eq 20 ]]; then
      echo "Timed out waiting for port-forward to become ready. See log at ${PORT_FORWARD_LOG}" >&2
      exit 1
    fi
  done
elif [[ -z "${BASE_URL}" ]]; then
  BASE_URL="http://127.0.0.1:8080"
fi

close_descriptions=(
  "AI assistant that produces concise, customer-facing summaries of support conversations."
  "Assistant focused on turning support chats into short customer-ready summaries."
  "Summarisation bot designed for service conversations, emphasizing actionable items."
  "Service desk summariser that creates brief recaps of customer interactions."
  "Conversation summariser tailored for customer support transcripts."
)

other_descriptions=(
  "Generates marketing headlines based on campaign goals and target personas."
  "Extracts named entities from compliance reports for downstream analytics."
  "Classifies product images into catalogue categories using vision embeddings."
  "Detects customer sentiment in real-time feedback streams."
  "Translates internal announcements between English and Spanish."
  "Matches user queries to relevant knowledge base articles."
  "Flags risky clauses in vendor contracts for legal review."
  "Generates step-by-step troubleshooting guides from incident descriptions."
  "Annotates meeting transcripts with action items and owners."
  "Summarises long-form blog posts into short social snippets."
  "Optimises SQL queries by suggesting index and rewrite strategies."
  "Rates call-center conversations for adherence to compliance scripts."
  "Analyzes code diffs to highlight potential security issues."
  "Creates personalised onboarding checklists for new employees."
  "Scores incoming tickets for urgency based on historical resolution data."
  "Produces FAQ answers from curated documentation sets."
  "Generates synthetic training data for classification workloads."
  "Monitors release notes and drafts internal status updates."
  "Clusters customer feedback into themed reports."
  "Suggests remediation steps for cloud infrastructure alerts."
)

TOTAL=20
echo "Seeding ${TOTAL} tools into ${BASE_URL} (owner=${TOOL_OWNER})"

create_tool() {
  local name="$1"
  local description="$2"
  local payload
  payload=$(jq -n \
    --arg owner "$TOOL_OWNER" \
    --arg name "$name" \
    --arg description "$description" \
    '{
      owner: $owner,
      name: $name,
      description: $description,
      inputs: { "text": "string" },
      outputs: { "result": "string" }
    }')

  response=$(curl -sS -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "${BASE_URL}/v1/tools")

  status=${response##*$'\n'}
  body=${response%$'\n'$status}

  if [[ "$status" != "201" ]]; then
    echo "Failed to create tool ${name} (status=${status}): ${body}" >&2
    exit 1
  fi

  tool_id=$(echo "$body" | jq -r '.tool.id')
  echo "✔ Created ${name} (${tool_id})"
}

# First five tools share similar descriptions for semantic grouping.
for i in "${!close_descriptions[@]}"; do
  seq=$((i + 1))
  create_tool "${TOOL_PREFIX}-summary-${seq}" "${close_descriptions[$i]}"
done

# Remaining tools use varied descriptions.
remaining=$((TOTAL - ${#close_descriptions[@]}))
for i in $(seq 1 "$remaining"); do
  description_index=$(( (i - 1) % ${#other_descriptions[@]} ))
  create_tool "${TOOL_PREFIX}-extra-${i}" "${other_descriptions[$description_index]}"
done

echo "Seeding complete."
