package vectorizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client calls an external embeddings service (e.g., Ollama, OpenAI-compatible).
type Client struct {
	baseURL        string
	model          string
	apiKey         string
	encodingFormat string
	httpClient     *http.Client
}

// New constructs a vectorizer client.
func New(baseURL, apiKey, model string) *Client {
	if baseURL == "" {
		return nil
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		model:          model,
		apiKey:         apiKey,
		encodingFormat: "float",
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Embed generates an embedding for free-form text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if c == nil {
		return nil, fmt.Errorf("vectorizer not configured")
	}
	payload := struct {
		Input          string `json:"input"`
		Model          string `json:"model"`
		EncodingFormat string `json:"encoding_format"`
	}{
		Input:          text,
		Model:          c.model,
		EncodingFormat: c.encodingFormat,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call vectorizer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vectorizer status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("vectorizer response missing data")
	}
	return result.Data[0].Embedding, nil
}
