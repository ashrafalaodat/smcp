package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultRegistryBaseURL = "http://mcp-registry.mcp.svc.cluster.local"
	registrySearchPath     = "/v1/tools/search"
	defaultSearchLimit     = 5
	requestTimeout         = 10 * time.Second
	maxResponseBytes       = 1 << 20 // 1 MiB
)

type searchArgs struct {
	Description string `json:"description"`
	Limit       int    `json:"limit,omitempty"`
}

type registrySearchRequest struct {
	Description string `json:"description"`
	Limit       int    `json:"limit,omitempty"`
}

type registryToolDetails struct {
	Tool registryTool `json:"tool"`
}

type registryTool struct {
	ID          string            `json:"id"`
	Owner       string            `json:"owner"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Inputs      map[string]string `json:"inputs"`
	Outputs     map[string]string `json:"outputs"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type searchResult struct {
	Query       string         `json:"query"`
	Limit       int            `json:"limit"`
	BaseURL     string         `json:"registry_base_url"`
	Tools       []registryTool `json:"tools"`
	MatchCount  int            `json:"match_count"`
	RequestedAt time.Time      `json:"requested_at"`
}

type invokeArgs struct {
	ToolID string         `json:"tool_id"`
	Input  map[string]any `json:"input,omitempty"`
}

type invokeResult struct {
	ToolID   string `json:"tool_id,omitempty"`
	Name     string `json:"tool_name"`
	Owner    string `json:"tool_owner"`
	Endpoint string `json:"endpoint"`
	Status   int    `json:"status"`
	Result   any    `json:"result,omitempty"`
	Raw      string `json:"raw,omitempty"`
}

var (
	httpClient = &http.Client{Timeout: requestTimeout}

	initOnce      sync.Once
	initErr       error
	registryURL   string
	mcpHTTPServer *server.StreamableHTTPServer
)

// Handle routes incoming Knative function requests to the MCP HTTP server.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	initOnce.Do(func() {
		initErr = initializeServer()
	})
	if initErr != nil {
		http.Error(res, fmt.Sprintf("initialization failed: %v", initErr), http.StatusInternalServerError)
		return
	}

	// Lightweight health endpoint for platform probes.
	if req.Method == http.MethodGet && (req.URL.Path == "/" || req.URL.Path == "/healthz") {
		res.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(res, "registry-mcp-search ready\n")
		return
	}

	// Delegate to MCP HTTP server for JSON-RPC interactions.
	mcpHTTPServer.ServeHTTP(res, req.Clone(ctx))
}

func initializeServer() error {
	registryURL = strings.TrimRight(getEnv("REGISTRY_BASE_URL", defaultRegistryBaseURL), "/")
	if registryURL == "" {
		return fmt.Errorf("registry base URL resolved to empty string")
	}

	serverName := getEnv("MCP_SERVER_NAME", "Registry Semantic Search")
	serverVersion := getEnv("MCP_SERVER_VERSION", "0.1.0")

	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(false),
	)

	tool := mcp.NewTool(
		"search",
		mcp.WithDescription("Search the MCP registry for tools using semantic similarity on descriptions."),
		mcp.WithString(
			"description",
			mcp.Description("Natural language description of the tool you are looking for."),
			mcp.Required(),
			mcp.MinLength(1),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Maximum number of tools to return. Optional; defaults to 5."),
			mcp.Min(1),
			mcp.Max(50),
			mcp.DefaultNumber(defaultSearchLimit),
		),
	)

	handler := mcp.NewTypedToolHandler(func(ctx context.Context, _ mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, error) {
		description := strings.TrimSpace(args.Description)
		if description == "" {
			return mcp.NewToolResultError("description is required"), nil
		}

		limit := args.Limit
		if limit <= 0 {
			limit = defaultSearchLimit
		}

		tools, err := callRegistrySearch(ctx, registryURL, description, limit)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("registry search failed", err), nil
		}

		result := searchResult{
			Query:       description,
			Limit:       limit,
			BaseURL:     registryURL,
			Tools:       tools,
			MatchCount:  len(tools),
			RequestedAt: time.Now().UTC(),
		}

		summary := buildSummary(description, tools)
		return mcp.NewToolResultStructured(result, summary), nil
	})

	s.AddTool(tool, handler)

	invokeToolDef := mcp.NewTool(
		"invoke",
		mcp.WithDescription("Invoke a discovered MCP tool using its registry tool_id."),
		mcp.WithString(
			"tool_id",
			mcp.Description("Tool identifier from the MCP registry."),
			mcp.Required(),
			mcp.MinLength(1),
		),
		mcp.WithAny(
			"input",
			mcp.Description("Arbitrary JSON payload forwarded to the tool."),
		),
	)

	invokeHandler := mcp.NewTypedToolHandler(func(ctx context.Context, _ mcp.CallToolRequest, args invokeArgs) (*mcp.CallToolResult, error) {
		invokeRes, err := invokeRegistryTool(ctx, registryURL, args)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invoke tool failed", err), nil
		}

		summary := fmt.Sprintf("Invoked %s/%s via %s (status %d).", invokeRes.Owner, invokeRes.Name, invokeRes.Endpoint, invokeRes.Status)
		return mcp.NewToolResultStructured(invokeRes, summary), nil
	})

	s.AddTool(invokeToolDef, invokeHandler)

	// Stateless sessions simplify client integration with shared Knative instances.
	mcpHTTPServer = server.NewStreamableHTTPServer(
		s,
		server.WithStateLess(true),
	)

	return nil
}

func callRegistrySearch(ctx context.Context, baseURL, description string, limit int) ([]registryTool, error) {
	payload := registrySearchRequest{
		Description: description,
		Limit:       limit,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	endpoint := baseURL + registrySearchPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var tools []registryTool
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return tools, nil
}

func invokeRegistryTool(ctx context.Context, baseURL string, args invokeArgs) (invokeResult, error) {
	toolID := strings.TrimSpace(args.ToolID)
	if toolID == "" {
		return invokeResult{}, fmt.Errorf("tool_id is required")
	}

	details, err := fetchRegistryTool(ctx, baseURL, toolID)
	if err != nil {
		return invokeResult{}, fmt.Errorf("resolve tool by id: %w", err)
	}

	toolName := details.Tool.Name
	toolOwner := details.Tool.Owner

	endpoint, err := formatToolEndpoint(toolName, toolOwner)
	if err != nil {
		return invokeResult{}, fmt.Errorf("derive endpoint: %w", err)
	}

	payload := map[string]any{
		"input": map[string]any{},
	}
	if len(args.Input) > 0 {
		payload["input"] = args.Input
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return invokeResult{}, fmt.Errorf("encode invocation payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return invokeResult{}, fmt.Errorf("build invocation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return invokeResult{}, fmt.Errorf("invoke request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return invokeResult{}, fmt.Errorf("read invoke response: %w", err)
	}

	result := invokeResult{
		ToolID:   toolID,
		Name:     toolName,
		Owner:    toolOwner,
		Endpoint: endpoint,
		Status:   resp.StatusCode,
	}

	if len(data) == 0 {
		return result, nil
	}

	if json.Valid(data) {
		var parsed any
		if err := json.Unmarshal(data, &parsed); err == nil {
			result.Result = parsed
			return result, nil
		}
	}

	result.Raw = string(data)
	return result, nil
}

func fetchRegistryTool(ctx context.Context, baseURL, toolID string) (registryToolDetails, error) {
	endpoint := fmt.Sprintf("%s/v1/tools/%s", strings.TrimRight(baseURL, "/"), toolID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return registryToolDetails{}, fmt.Errorf("build registry request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return registryToolDetails{}, fmt.Errorf("registry request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return registryToolDetails{}, fmt.Errorf("read registry response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return registryToolDetails{}, fmt.Errorf("registry returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var details registryToolDetails
	if err := json.Unmarshal(data, &details); err != nil {
		return registryToolDetails{}, fmt.Errorf("decode registry response: %w", err)
	}
	return details, nil
}

func formatToolEndpoint(name, owner string) (string, error) {
	safeName := sanitizeSubdomain(name)
	safeOwner := sanitizeSubdomain(owner)
	if safeName == "" || safeOwner == "" {
		return "", fmt.Errorf("cannot derive endpoint for name=%q owner=%q", name, owner)
	}
	return fmt.Sprintf("http://%s.%s.localhost", safeName, safeOwner), nil
}

func sanitizeSubdomain(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}

	var b strings.Builder
	lastHyphen := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastHyphen = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_' || r == ' ' || r == '.' || r == '/':
			if !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		default:
			// skip unsupported characters
		}
	}

	return strings.Trim(b.String(), "-")
}

func buildSummary(query string, tools []registryTool) string {
	if len(tools) == 0 {
		return fmt.Sprintf("No tools matched the description %q.", query)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d tool(s) for %q:\n", len(tools), query))
	for i, tool := range tools {
		description := shorten(tool.Description, 160)
		b.WriteString(fmt.Sprintf("%d. %s (%s) — %s\n",
			i+1,
			fallback(tool.Name, "unnamed tool"),
			fallback(tool.Owner, "unknown owner"),
			description,
		))
	}
	return strings.TrimRight(b.String(), "\n")
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func shorten(input string, max int) string {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) <= max {
		return trimmed
	}
	if max <= 1 {
		return trimmed[:max]
	}
	return trimmed[:max-1] + "…"
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func init() {
	if httpClient.Timeout <= 0 {
		httpClient.Timeout = requestTimeout
	}
}
