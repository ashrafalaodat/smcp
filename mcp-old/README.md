# s2mcp Function

Knative function that discovers Model Context Protocol tools via the registry service and proxies invocation calls on behalf of the caller.

## Actions

- `discover`: Provide `query` and/or `tags` to fetch matching tools. Returns proxy metadata so the consumer can address the function as the tool endpoint.
- `invoke`: Include `tool.id` and optional `tool.original_endpoint` (defaults to the registry server endpoint) and any `tool.input`. The function fetches the latest registry snapshot, verifies the tool, forwards the invocation to the MCP server, and returns the downstream response.

Set `REGISTRY_BASE_URL` if the registry is exposed on a custom URL. Defaults to `http://mcp-registry.default.svc.cluster.local`.
