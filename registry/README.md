# MCP Registry Service

A standalone discovery catalog for Model Context Protocol (MCP) tools. Each tool entry captures ownership, free-form metadata, structured input/output descriptors, semantic embeddings, policies, and audit history. The service exposes a REST API, persists data in PostgreSQL (with pgvector), and ships with Helm charts for both the app and its backing database.

## Features
- Go chi-based HTTP API for tool registration, lookup, and audit logging (`POST /v1/tools`, `GET /v1/tools`, `GET /v1/tools/{id}`, `POST /v1/tools/{id}/audits`).
- Automatic text embeddings via an external vectorization microservice (configurable URL/API key) stored with `pgvector` for semantic discovery.
- Policy management tied to each tool with JSON condition payloads.
- Structured logging (zap) and optional Prometheus metrics endpoint.
- Helm charts for application and PostgreSQL (with init scripts installing required extensions and schema).

## Build & Run

### Prerequisites
- Go 1.22+
- PostgreSQL 14+ with `uuid-ossp`, `pg_trgm`, `vector` extensions (the provided Helm chart enables these)

```bash
export MCP_REGISTRY_DATABASE_URL="postgres://registry:secret@localhost:5432/registry?sslmode=disable"
GOCACHE=/tmp/go-build go build ./...
MCP_REGISTRY_HTTP_ADDRESS=":8080" \
GOCACHE=/tmp/go-build go run ./cmd/registry
```

Environment variables (prefixed `MCP_REGISTRY_`):

| Variable | Description | Default |
| --- | --- | --- |
| `SERVICE_NAME` | Service identifier used in logs | `mcp-registry` |
| `HTTP_ADDRESS` | Bind address for HTTP API | `:8080` |
| `GRACEFUL_SHUTDOWN_TIMEOUT` | Grace period for shutdown | `20s` |
| `DATABASE_URL` | PostgreSQL DSN | _required_ |
| `VECTORIZE_URL` | Base URL for the vectorization microservice | `http://localhost:11434` |
| `VECTORIZE_API_KEY` | Optional bearer token for vectorizer requests | _(unset)_ |
| `VECTORIZE_MODEL` | Embedding model to request from the vectorizer | `nomic-embed-text` |
| `METRICS_ENABLED` | Expose `/metrics` | `true` |
| `LOG_LEVEL` | Structured log level (`debug`,`info`,`warn`,`error`) | `info` |

Override the vectorizer settings if you run a different embeddings service or model.

## Docker

```bash
docker build -t ashrafalaodat/mcp-registry:latest .

docker run --rm \
  -p 8080:8080 \
  -e MCP_REGISTRY_DATABASE_URL="postgres://registry:secret@host.docker.internal:5432/registry?sslmode=disable" \
  ashrafalaodat/mcp-registry:latest
```

The image is multi-stage: a static binary compiled in `golang:alpine` and packaged into `distroless` for a minimal runtime surface.

## Kubernetes Deployment
1. Provision PostgreSQL with the bundled chart:

    ```bash
    helm upgrade --install mcp-registry-db postgres/charts \
      --set primary.database=registry \
      --set primary.username=registry
    ```

    The chart creates a secret named `<release>-postgres` (for example `mcp-registry-db-postgres`) containing connection data (`username`, `password`, `database`, `host`, `port`, `uri`).

2. Deploy the registry:

    ```bash
    helm upgrade --install mcp-registry registry/charts \
      --set image.repository=ghcr.io/ashrafalaodat/mcp-registry \
      --set image.tag=latest \
      --set env[0].valueFrom.secretKeyRef.name=mcp-registry-db-postgres \
      --set env[0].valueFrom.secretKeyRef.key=uri
    ```

3. (Optional) The equivalent raw manifests mirror the chart (Deployment/Service exposing port 8080).

4. Front the service with an Ingress or mesh gateway and secure it with TLS/mTLS.

## Database Schema

The schema managed by the init scripts and migrations:

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE tools (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  owner TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  embedding vector,
  inputs JSONB DEFAULT '{}'::jsonb,
  outputs JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE policies (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tool_id UUID REFERENCES tools(id) ON DELETE CASCADE,
  principal TEXT NOT NULL,
  allowed_scope TEXT[] DEFAULT '{}',
  conditions JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE audit (
  id BIGSERIAL PRIMARY KEY,
  tool_id UUID REFERENCES tools(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  actor TEXT NOT NULL,
  payload JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tools_name_trgm ON tools USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_tools_description_trgm ON tools USING gin (description gin_trgm_ops);
```

## API Quick Reference

```http
POST /v1/tools
Content-Type: application/json

{
  "owner": "team-search",
  "name": "vector-summarize",
  "description": "Summarises documents",
  "inputs": {"text": "string"},
  "outputs": {"summary": "string"},
  "policies": [
    {
      "principal": "service:agent",
      "allowed_scope": ["summaries"],
      "conditions": {"region": "us-east"}
    }
  ]
}
```
Creates/updates a tool (embeddings are generated automatically when the vectorizer is configured). Returns the tool with attached policies and recent audits.

```http
GET /v1/tools?owner=team-search&q=summarize
```
Lists tools filtered by owner/name/query.

```http
GET /v1/tools/{id}
```
Retrieves a single tool with policies and recent audit events.

```http
POST /v1/tools/{id}/audits
Content-Type: application/json

{
  "event_type": "invocation",
  "actor": "agent/42",
  "payload": {"status": "success"}
}
```
Appends an audit record to a tool.

## Development Notes
- Run `GOCACHE=/tmp/go-build go test ./...` to exercise unit tests once added.
- Use the provided Postgres Helm chart or Docker Compose for local development; Postgres must have `pgvector` enabled.
- Extend the service and repository layers to support semantic search or similarity queries leveraging the stored embeddings.
