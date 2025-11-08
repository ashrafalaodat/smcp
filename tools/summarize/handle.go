package function

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultOllamaBaseURL = "http://ollama.mcp.svc.cluster.local:11434"
	defaultOllamaModel   = "llama3"
	maxRequestBodySize   = 1 << 20 // 1 MiB
)

var (
	ollamaBaseURL    = strings.TrimRight(getEnv("OLLAMA_BASE_URL", defaultOllamaBaseURL), "/")
	ollamaModel      = getEnv("OLLAMA_MODEL", defaultOllamaModel)
	ollamaHTTPClient = &http.Client{
		Timeout: 5 * time.Minute,
	}
)

type ollamaGenerateRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	Stream  bool          `json:"stream"`
	Options ollamaOptions `json:"options,omitempty"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

type ollamaOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// Handle summarizes the request body using an Ollama instance and returns the summary.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(res, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer req.Body.Close()

	body, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodySize))
	if err != nil {
		http.Error(res, "failed to read request body", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(string(body))
	if text == "" {
		http.Error(res, "request body must contain text to summarize", http.StatusBadRequest)
		return
	}

	summary, err := summarize(ctx, text)
	if err != nil {
		fmt.Println("summarize error:", err)
		http.Error(res, "failed to generate summary", http.StatusBadGateway)
		return
	}

	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	res.WriteHeader(http.StatusOK)
	_, _ = res.Write([]byte(summary))
}

func summarize(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("text is empty")
	}

	payload := ollamaGenerateRequest{
		Model:  ollamaModel,
		Prompt: fmt.Sprintf("Provide a concise summary of the following text. Keep it to two sentences.\n\n%s", text),
		Stream: false,
		Options: ollamaOptions{
			NumPredict:  80,
			Temperature: 0.2,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/generate", strings.TrimRight(ollamaBaseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	summary := strings.TrimSpace(decoded.Response)
	if summary == "" {
		return "", errors.New("ollama returned an empty summary")
	}

	return summary, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
