package function

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDiscoverMatches(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[
			{
				"server": {
					"id": "server-123",
					"name": "Weather Service",
					"endpoint": "http://weather.example.com/invoke",
					"version": "1.0.0",
					"region": "global",
					"auth_method": "none",
					"metadata": {"description": "Weather data"}
				},
				"capabilities": [
					{
						"id": "cap-456",
						"server_id": "server-123",
						"type": "tool",
						"name": "weather",
						"schema": {"input": "city"},
						"tags": ["weather", "forecast"],
						"created_at": "2025-01-01T00:00:00Z"
					}
				]
			}
		]`)
	}))
	defer registry.Close()

	t.Setenv("REGISTRY_BASE_URL", registry.URL)

	reqBody := `{"action":"discover","query":"current weather in amman"}`
	req := httptest.NewRequest(http.MethodPost, "http://s2mcp.default.localhost/", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	Handle(context.Background(), rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp discoveryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Matched {
		t.Fatalf("expected matched=true, got false")
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(resp.Tools))
	}
	tool := resp.Tools[0]
	if tool.ProxyEndpoint != "http://s2mcp.default.localhost" {
		t.Fatalf("unexpected proxy endpoint %q", tool.ProxyEndpoint)
	}
	if tool.OriginalEndpoint != "http://weather.example.com/invoke" {
		t.Fatalf("unexpected original endpoint %q", tool.OriginalEndpoint)
	}
	if tool.ID != "server-123:cap-456" {
		t.Fatalf("unexpected tool id %q", tool.ID)
	}
	if tool.Score <= 0 {
		t.Fatalf("expected positive score, got %f", tool.Score)
	}
}

func TestHandleDiscoverNoMatch(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[
			{
				"server": {
					"id": "server-999",
					"name": "Math Service",
					"endpoint": "http://math.example.com",
					"version": "1.2.3",
					"region": "global",
					"auth_method": "none",
					"metadata": {}
				},
				"capabilities": [
					{
						"id": "cap-abc",
						"server_id": "server-999",
						"type": "tool",
						"name": "calculator",
						"schema": null,
						"tags": ["math"],
						"created_at": "2025-01-01T00:00:00Z"
					}
				]
			}
		]`)
	}))
	defer registry.Close()

	t.Setenv("REGISTRY_BASE_URL", registry.URL)

	reqBody := `{"action":"discover","query":"latest weather outlook"}`
	req := httptest.NewRequest(http.MethodPost, "http://s2mcp.default.localhost/", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	Handle(context.Background(), rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp discoveryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Matched {
		t.Fatalf("expected matched=false, got true")
	}
	if len(resp.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(resp.Tools))
	}
}

func TestHandleDiscoverTopFive(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[
		{
			"server": {
				"id": "server-forecast",
				"name": "Forecast Hub",
				"endpoint": "http://forecast.example.com",
				"version": "2.0.0",
				"region": "global",
				"auth_method": "none",
				"metadata": {"description": "Comprehensive weather toolkit"}
			},
			"capabilities": [
				{"id":"cap-1","server_id":"server-forecast","type":"tool","name":"weather-alpha","schema":{"description":"weather forecast humidity rain temperature climate storm alerts"},"tags":["weather","forecast"],"created_at":"2025-01-01T00:00:00Z"},
				{"id":"cap-2","server_id":"server-forecast","type":"tool","name":"weather-beta","schema":{"description":"weather forecast humidity rain temperature"},"tags":["weather"],"created_at":"2025-01-01T00:00:00Z"},
				{"id":"cap-3","server_id":"server-forecast","type":"tool","name":"weather-gamma","schema":{"description":"weather forecast rain"},"tags":["weather"],"created_at":"2025-01-01T00:00:00Z"},
				{"id":"cap-4","server_id":"server-forecast","type":"tool","name":"weather-delta","schema":{"description":"forecast climate data"},"tags":["weather"],"created_at":"2025-01-01T00:00:00Z"},
				{"id":"cap-5","server_id":"server-forecast","type":"tool","name":"weather-epsilon","schema":{"description":"humidity alert system"},"tags":["weather"],"created_at":"2025-01-01T00:00:00Z"},
				{"id":"cap-6","server_id":"server-forecast","type":"tool","name":"weather-zeta","schema":{"description":"general conditions"},"tags":["weather"],"created_at":"2025-01-01T00:00:00Z"}
			]
		}
	]`)
	}))
	defer registry.Close()

	t.Setenv("REGISTRY_BASE_URL", registry.URL)

	reqBody := `{"action":"discover","query":"weather rain humidity temperature forecast alerts climate storms"}`
	req := httptest.NewRequest(http.MethodPost, "http://s2mcp.default.localhost/", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	Handle(context.Background(), rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp discoveryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Tools) != 5 {
		t.Fatalf("expected top 5 tools, got %d", len(resp.Tools))
	}

	names := map[string]struct{}{}
	for _, tool := range resp.Tools {
		names[tool.Name] = struct{}{}
	}
	expectedFirst := "weather-alpha"
	if resp.Tools[0].Name != expectedFirst {
		t.Fatalf("expected top ranked tool %q, got %q", expectedFirst, resp.Tools[0].Name)
	}
	for i := 1; i < len(resp.Tools); i++ {
		if resp.Tools[i].Score > resp.Tools[i-1].Score {
			t.Fatalf("scores not sorted descending: %f before %f", resp.Tools[i-1].Score, resp.Tools[i].Score)
		}
	}
	for _, tool := range resp.Tools {
		if tool.Score <= 0 {
			t.Fatalf("expected positive score for %q, got %f", tool.Name, tool.Score)
		}
	}
	for _, name := range []string{"weather-alpha", "weather-beta", "weather-gamma", "weather-delta", "weather-epsilon"} {
		if _, ok := names[name]; !ok {
			t.Fatalf("expected tool %q in top results", name)
		}
	}
	if _, ok := names["weather-zeta"]; ok {
		t.Fatalf("unexpected tool weather-zeta in top results")
	}
}

func TestHandleInvokeProxies(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[
			{
				"server": {
					"id": "server-123",
					"name": "Weather Service",
					"endpoint": "http://placeholder",
					"version": "1.0.0",
					"region": "global",
					"auth_method": "none",
					"metadata": {}
				},
				"capabilities": [
					{
						"id": "cap-456",
						"server_id": "server-123",
						"type": "tool",
						"name": "weather",
						"schema": null,
						"tags": ["weather"],
						"created_at": "2025-01-01T00:00:00Z"
					}
				]
			}
		]`)
	}))
	defer registry.Close()

	var received map[string]any
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"outcome":"sunny"}`)
	}))
	defer remote.Close()

	t.Setenv("REGISTRY_BASE_URL", registry.URL)

	reqBody := fmt.Sprintf(`{
		"action":"invoke",
		"tool":{
			"id":"server-123:cap-456",
			"original_endpoint":"%s",
			"input":{"city":"amman"}
		}
	}`, remote.URL)

	req := httptest.NewRequest(http.MethodPost, "http://s2mcp.default.localhost/", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	Handle(context.Background(), rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	if received == nil {
		t.Fatalf("remote server did not receive a payload")
	}

	var resp invokeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode invoke response: %v", err)
	}

	if !resp.Proxied {
		t.Fatalf("expected proxied=true")
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("expected proxied status 200, got %d", resp.Status)
	}
	if resp.Result == nil {
		t.Fatalf("expected proxied result")
	}

	toolSection, ok := received["tool"].(map[string]any)
	if !ok {
		t.Fatalf("remote payload missing tool section: %#v", received)
	}
	if toolSection["id"] != "server-123:cap-456" {
		t.Fatalf("unexpected tool id forwarded: %v", toolSection["id"])
	}
	inputSection, ok := toolSection["input"].(map[string]any)
	if !ok {
		t.Fatalf("remote payload missing input section: %#v", toolSection)
	}
	if inputSection["city"] != "amman" {
		t.Fatalf("unexpected forwarded input: %#v", inputSection)
	}
}
