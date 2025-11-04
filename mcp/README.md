# Registry MCP Search Function

Knative function that serves a Model Context Protocol endpoint backed by the registry service's semantic search API.

Incoming HTTP requests are handled by `mark3labs/mcp-go` using the streamable HTTP transport, exposing a single tool named `search`.

## Configuration

| Variable              | Default                                        | Description                                                      |
|-----------------------|------------------------------------------------|------------------------------------------------------------------|
| `REGISTRY_BASE_URL`   | `http://mcp-registry.mcp.svc.cluster.local` | Base URL of the app registry. Must expose `/v1/tools/search`.    |
| `MCP_SERVER_NAME`     | `Registry Semantic Search`                      | Display name reported during MCP `initialize`.                   |
| `MCP_SERVER_VERSION`  | `0.1.0`                                         | Version string reported during MCP `initialize`.                 |

## Local Development

```bash
# Deploy function image with Knative Func CLI
func deploy --path .

# Alternatively, build locally
func build --builder pack --path .

# Invoke health probe
curl http://<route>/healthz
```

## Tool: `search`

- `description` *(required, string)* – Natural language description of the tool you are looking for.
- `limit` *(optional, number, default 5)* – Maximum number of results to return (clamped between 1 and 50).

Results include the query metadata, registry base URL, and the list of matched tools. A textual summary is provided as the fallback response.
