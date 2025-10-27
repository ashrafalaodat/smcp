# MCP Registry Service

A standalone discovery and policy registry for Model Context Protocol (MCP) servers.  
It exposes a REST API for registering MCP services, reporting heartbeats, and discovering capabilities/policies backed by PostgreSQL (with optional Redis caching and Prometheus metrics).

## Features
- Go chi-based HTTP API (`POST /v1/servers`, `PUT /v1/servers/{id}/heartbeat`, `GET /v1/servers`).
- PostgreSQL persistence with room for extensions (pgvector, TimescaleDB).
- Structured logging via `zap` and optional Prometheus metrics endpoint.
- Ready for Kubernetes deployment as a standard Deployment + Service.

## Build & Run

### Prerequisites
- Go 1.22+
- PostgreSQL 14+ (with `uuid-ossp` extension enabled)

```bash
export MCP_REGISTRY_DATABASE_URL="postgres://registry:secret@localhost:5432/registry?sslmode=disable"
GOCACHE=/tmp/go-build go build ./...
MCP_REGISTRY_HTTP_ADDRESS=":8080" \
GOCACHE=/tmp/go-build go run ./cmd/registry
```

Environment variables are read with the `MCP_REGISTRY_` prefix:

| Variable | Description | Default |
| --- | --- | --- |
| `MCP_REGISTRY_SERVICE_NAME` | Service identifier used in logs | `mcp-registry` |
| `MCP_REGISTRY_HTTP_ADDRESS` | Bind address for HTTP API | `:8080` |
| `MCP_REGISTRY_GRACEFUL_SHUTDOWN_TIMEOUT` | Grace period for shutdown | `20s` |
| `MCP_REGISTRY_DATABASE_URL` | PostgreSQL DSN | _required_ |
| `MCP_REGISTRY_REDIS_ENABLED` | Enable Redis caching | `false` |
| `MCP_REGISTRY_REDIS_ADDR` | Redis host:port | `127.0.0.1:6379` |
| `MCP_REGISTRY_METRICS_ENABLED` | Expose `/metrics` | `true` |
| `MCP_REGISTRY_LOG_LEVEL` | Structured log level (`debug`,`info`,`warn`,`error`) | `info` |

## Docker

```bash
docker build -t ashrafalaodat/mcp-registry:latest .

docker run --rm \
  -p 8080:8080 \
  -e MCP_REGISTRY_DATABASE_URL="postgres://registry:secret@host.docker.internal:5432/registry?sslmode=disable" \
  ashrafalaodat/mcp-registry:latest
```

The image is multi-stage: a static binary compiled in `golang:alpine` and packaged into `distroless` for minimal surface area.

## Kubernetes Deployment
1. Provision PostgreSQL via `StatefulSet` with `uuid-ossp`, `pgvector` extensions as needed.
2. Deploy the PostgreSQL chart in `../postgres/charts`:

```bash
helm upgrade --install mcp-registry-db postgres/charts \
  --set primary.database=registry \
  --set primary.username=registry
```

3. The PostgreSQL chart creates a Secret named `RELEASE-NAME-postgres` (for example `mcp-registry-db-postgres`) with connection details under the keys `username`, `password`, `database`, `host`, `port`, and `uri`.
4. Deploy with the provided Helm chart (update the secret name if your PostgreSQL release is named differently):

```bash
helm upgrade --install mcp-registry ./registry/charts \
    --set image.repository=mcp-registry \
    --set image.tag=latest \
    --set securityContext.runAsUser=65532 \
    --set securityContext.runAsGroup=65532 \
    --set securityContext.runAsNonRoot=true \
    --set securityContext.readOnlyRootFilesystem=true
```

4. (Optional) If you prefer raw manifests, an equivalent Deployment/Service looks like:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-registry
spec:
  replicas: 2
  selector:
    matchLabels: { app: mcp-registry }
  template:
    metadata:
      labels: { app: mcp-registry }
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      containers:
        - name: registry
          image: ashrafalaodat/mcp-registry:latest
          ports: [{ containerPort: 8080 }]
          env:
            - name: MCP_REGISTRY_DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: mcp-registry-db-postgres
                  key: uri
          readinessProbe:
            httpGet:
              path: /v1/servers
              port: 8080
          livenessProbe:
            httpGet:
              path: /v1/servers
              port: 8080

---
apiVersion: v1
kind: Service
metadata:
  name: mcp-registry
spec:
  selector: { app: mcp-registry }
  ports:
    - port: 80
      targetPort: 8080
```

5. Expose via `Ingress` or Service Mesh (Istio/Linkerd) and protect with TLS/mTLS.

## Database Schema

Create tables with your migration tool of choice (e.g., `goose`, `golang-migrate`):

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE servers (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  version TEXT,
  region TEXT,
  auth_method TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE capabilities (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  schema JSONB DEFAULT '{}'::jsonb,
  tags TEXT[],
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE policies (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
  principal TEXT NOT NULL,
  allowed_tools TEXT[],
  conditions JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE health (
  server_id UUID PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  last_seen TIMESTAMPTZ,
  status TEXT,
  latency_ms INT,
  error_rate NUMERIC,
  details JSONB DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE audit (
  id BIGSERIAL PRIMARY KEY,
  server_id UUID REFERENCES servers(id),
  tool_name TEXT,
  event_type TEXT,
  actor TEXT,
  payload JSONB,
  created_at TIMESTAMPTZ NOT NULL
);
```

## API Quick Reference

```http
POST /v1/servers
Content-Type: application/json
```
Registers or updates an MCP server. Include capabilities/policies arrays to replace existing ones.

```http
PUT /v1/servers/{id}/heartbeat
Content-Type: application/json
```
Reports health metrics and updates `last_seen`/`status`.

```http
GET /v1/servers?region=us-east&status=online&tag=tooling
```
Fetches enriched server snapshots filtered by region/status/tag.

## Development Tips
- Use `docker compose` with Postgres + Redis for local integration testing.
- Optionally wire `Testcontainers` in Go to exercise repository/service layers.
- Consider adding gRPC/GraphQL endpoints if you need richer discovery semantics.
