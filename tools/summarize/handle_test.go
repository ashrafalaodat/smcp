package function

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSummarizesRequestBody(t *testing.T) {
	var receivedPrompt string
	const summary = "summary response"
	const modelName = "test-model"

	// Stub Ollama endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var payload ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}

		if payload.Model != modelName {
			t.Fatalf("expected model %q, got %q", modelName, payload.Model)
		}
		receivedPrompt = payload.Prompt

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ollamaGenerateResponse{Response: summary}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	restore := stubOllamaClient(server, modelName)
	defer restore()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/summarize", strings.NewReader("Important details go here."))

	Handle(context.Background(), w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if got := string(body); got != summary {
		t.Fatalf("expected summary %q, got %q", summary, got)
	}

	if !strings.Contains(receivedPrompt, "Important details go here.") {
		t.Fatalf("prompt does not include original text: %q", receivedPrompt)
	}
}

func TestHandleRejectsEmptyBodies(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/summarize", strings.NewReader("   "))

	Handle(context.Background(), w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", res.StatusCode)
	}
}

func TestHandleRejectsInvalidMethod(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/summarize", nil)

	Handle(context.Background(), w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", res.StatusCode)
	}
}

func stubOllamaClient(server *httptest.Server, modelName string) func() {
	prevURL := ollamaBaseURL
	prevClient := ollamaHTTPClient
	prevModel := ollamaModel

	ollamaBaseURL = server.URL
	ollamaHTTPClient = server.Client()
	ollamaModel = modelName

	return func() {
		ollamaBaseURL = prevURL
		ollamaHTTPClient = prevClient
		ollamaModel = prevModel
	}
}
