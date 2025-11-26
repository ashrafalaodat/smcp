package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ToolPayload matches the registry POST body.
type ToolPayload struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Visibility  string `json:"visibility"`
	Description string `json:"description"`
}

// Client performs HTTP calls to the registry API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewHTTPClient builds a registry client.
func NewHTTPClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

// Upsert registers or updates a tool.
func (c *Client) Upsert(ctx context.Context, payload ToolPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/tools", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry upsert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("registry upsert status %d", resp.StatusCode)
}

// Delete removes a tool.
func (c *Client) Delete(ctx context.Context, owner, name string) error {
	url := fmt.Sprintf("%s/v1/tools/%s/%s", c.baseURL, owner, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("registry delete status %d", resp.StatusCode)
}
