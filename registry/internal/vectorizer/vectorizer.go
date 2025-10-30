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

// Client calls an external vectorization microservice.
type Client struct {
	baseURL        string
	model          string
	apiKey         string
	encodingFormat string
	client         *http.Client
}

// New creates a vectorizer client.
func New(baseURL, apiKey, model string) *Client {
	const (
		defaultModel = "nomic-embed-text"
	)

	if model == "" {
		model = defaultModel
	}

	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		model:          model,
		apiKey:         apiKey,
		encodingFormat: "float",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Embed generates an embedding for the provided text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("vectorizer not configured")
	}

	type embedRequest struct {
		Input          string `json:"input"`
		Model          string `json:"model"`
		EncodingFormat string `json:"encoding_format"`
	}

	payload := embedRequest{
		Input:          text,
		Model:          c.model,
		EncodingFormat: c.encodingFormat,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	endpoint := c.baseURL + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call vectorizer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vectorizer returned status %d", resp.StatusCode)
	}

	type embedding struct {
		Embedding []float32 `json:"embedding"`
	}
	var result struct {
		Data []embedding `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode vectorizer response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("vectorizer response missing data")
	}

	return result.Data[0].Embedding, nil
}
