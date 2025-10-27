#!/usr/bin/env bash

set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}

echo ">>> POST /v1/servers"
curl -i -X POST "${BASE_URL}/v1/servers" \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "agent-suite",
    "display_name": "Agent Suite",
    "endpoint": "https://agents.example.com",
    "region": "us-east",
    "tags": ["tooling", "beta"],
    "capabilities": [
      "discovery.search",
      "messaging.broadcast"
    ],
    "policies": [
      { "name": "max-session-duration", "value": "4h" },
      { "name": "allow-listed-clients", "value": ["cli", "web"] }
    ]
  }'

echo -e "\n\n>>> PUT /v1/servers/agent-suite/heartbeat"
curl -i -X PUT "${BASE_URL}/v1/servers/agent-suite/heartbeat" \
  -H 'Content-Type: application/json' \
  -d '{
    "status": "online",
    "latency_ms": 87,
    "error_rate": 0.002,
    "active_sessions": 128,
    "capacity": { "cpu": 0.68, "memory": 0.55 }
  }'

echo -e "\n\n>>> GET /v1/servers?region=us-east&status=online&tag=tooling"
curl -i "${BASE_URL}/v1/servers?region=us-east&status=online&tag=tooling"

echo
