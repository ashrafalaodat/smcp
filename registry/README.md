# MCP Registry (Qdrant Edition)

This implementation keeps the registry surface minimal: each tool consists of an owner, a tool name, and the embedding vector that represents its behavior. The registry never accepts raw embeddings—instead callers send a natural-language description and the service uses the configured vectorizer to generate the embedding before persisting it in [Qdrant](https://qdrant.tech). Metadata such as `visibility` (e.g., `public` vs `private`) lives alongside the vector to enable filtering.

## Why Qdrant?
- **GPU index support** via product quantization / HNSW graphs enables low-latency similarity search for high-dimensional embeddings without managing pgvector extensions yourself.
- **Payload filters** allow us to enforce visibility (public/private) or owner scoping without adding more columns to the logical data model.
- **Operational simplicity**: a single GRPC endpoint backs both collection management and ANN queries.

## Data Model
| Field        | Type        | Notes |
|--------------|-------------|-------|
| `owner`      | string      | Partition key used in metadata filters. |
| `tool_name`  | string      | Secondary identifier inside an owner namespace. |
| `embedding`  | float vector| Fixed size (`MCP_QDRANT_EMBEDDING_DIM`), generated at write time from the submitted description. |
| `visibility` | string      | Optional metadata (`public`/`private`). |

A deterministic UUID derived from `owner::tool_name` becomes the Qdrant point ID; the payload stores the three logical fields and visibility metadata.

## API Surface
```
POST   /v1/tools          # register/update (description required, embedding auto-generated)
GET    /v1/tools          # list tools (filter by owner/visibility)
GET    /v1/tools/{owner}/{name}
DELETE /v1/tools/{owner}/{name}
POST   /v1/tools/search   # ANN search with optional filters/visibility
```

Responses echo the stored embedding so downstream agents can reuse vectors without re-querying Qdrant.

## Running Locally
1. **Start Qdrant** (CPU or GPU, example below sticks to CPU):
   ```bash
   docker run -p 6333:6333 -p 6334:6334 \
     -e QDRANT__SERVICE__API_KEY=secret \
     -e QDRANT__STORAGE__USE_SCALAR_QUANTIZATION=true \
     -e QDRANT__STORAGE__ON_DISK_VECTORS=false \
     qdrant/qdrant:latest
   ```
2. **Configure the registry** (minimal env vars):
   ```bash
   export MCP_QDRANT_HOST=localhost
   export MCP_QDRANT_PORT=6334
   export MCP_QDRANT_EMBEDDING_DIM=1536
   export MCP_QDRANT_VECTORIZE_URL="http://localhost:11434"
   ```
   > The vectorizer endpoint (Ollama, OpenAI-compatible service, etc.) is mandatory because all embeddings are generated server-side.
3. **Run**:
   ```bash
   cd registry
   GOCACHE=/tmp/go-build go run ./cmd/registry
   ```

On startup the service ensures the `mcp_tools` collection exists (creating it with GPU-friendly HNSW + product quantization defaults if needed), then exposes the HTTP API on `:8080`.

### Example: Registering a Tool

```bash
curl -X POST http://localhost:8080/v1/tools \
  -H 'Content-Type: application/json' \
  -d '{
    "owner": "demo",
    "name": "summarizer",
    "visibility": "public",
    "description": "Summarises customer tickets into short action items"
  }'
```

The registry calls the vectorizer with the provided description, verifies the returned embedding matches `MCP_QDRANT_EMBEDDING_DIM`, and stores owner/name/embedding inside Qdrant.

## Helm Charts
- `registry/charts` deploys the application (Deployment + Service) and wires the `MCP_QDRANT_*` environment for the binary.
- `qdrant/charts` deploys a GPU-enabled Qdrant StatefulSet with persistent storage so you can keep the ANN index near the registry.
