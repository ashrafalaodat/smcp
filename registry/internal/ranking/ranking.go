package ranking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ashrafalaodat/registry/internal/domain"
)

// Client calls an external reranking API (e.g. Cohere) to reorder semantic search results.
type Client struct {
	endpoint   string
	apiKey     string
	model      string
	topN       int
	httpClient *http.Client
}

// NewClient constructs a reranker client. topN controls how many candidates the remote model should score.
func NewClient(endpoint, apiKey, model string, topN int) *Client {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}

	if topN <= 0 {
		topN = 20
	}

	return &Client{
		endpoint: endpoint,
		apiKey:   strings.TrimSpace(apiKey),
		model:    model,
		topN:     topN,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Rank reorders the provided candidates using the configured reranking endpoint.
func (c *Client) Rank(ctx context.Context, query string, candidates []domain.Tool) ([]domain.Tool, error) {
	if c == nil || len(candidates) == 0 {
		return candidates, nil
	}

	request := rerankRequest{
		Query: query,
		Model: c.model,
		TopN:  clampTopN(c.topN, len(candidates)),
	}

	request.Documents = make([]rerankDocument, 0, len(candidates))
	for _, tool := range candidates {
		request.Documents = append(request.Documents, rerankDocument{
			ID:   tool.ID.String(),
			Text: buildDocumentText(tool),
		})
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call rerank endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank endpoint returned status %d", resp.StatusCode)
	}

	var result rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}

	if len(result.Results) == 0 {
		return candidates, nil
	}

	ranked := make([]domain.Tool, 0, len(candidates))
	seen := make(map[int]bool, len(result.Results))

	for _, entry := range result.Results {
		if entry.Index < 0 || entry.Index >= len(candidates) {
			continue
		}
		tool := candidates[entry.Index]
		ranked = append(ranked, tool)
		seen[entry.Index] = true
		if len(ranked) >= len(candidates) {
			break
		}
	}

	for idx, tool := range candidates {
		if seen[idx] {
			continue
		}
		ranked = append(ranked, tool)
	}

	return ranked, nil
}

type rerankDocument struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text"`
}

type rerankRequest struct {
	Query     string           `json:"query"`
	Model     string           `json:"model,omitempty"`
	TopN      int              `json:"top_n,omitempty"`
	Documents []rerankDocument `json:"documents"`
}

type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

func clampTopN(topN, candidateCount int) int {
	if topN <= 0 || topN > candidateCount {
		return candidateCount
	}
	return topN
}

func buildDocumentText(tool domain.Tool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s / %s\n", tool.Owner, tool.Name))
	b.WriteString(tool.Description)

	if len(tool.Inputs) > 0 {
		b.WriteString("\nInputs:")
		for key, value := range tool.Inputs {
			b.WriteString(fmt.Sprintf(" %s=%s;", key, value))
		}
	}

	if len(tool.Outputs) > 0 {
		b.WriteString("\nOutputs:")
		for key, value := range tool.Outputs {
			b.WriteString(fmt.Sprintf(" %s=%s;", key, value))
		}
	}

	return strings.TrimSpace(b.String())
}
