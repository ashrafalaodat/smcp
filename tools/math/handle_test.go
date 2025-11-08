package function

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleJSONBody(t *testing.T) {
	body := `{"a":3,"b":4,"op":"multiply"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/math", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	Handle(context.Background(), rec, req)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var payload mathResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Result != 12 {
		t.Fatalf("expected result 12, got %v", payload.Result)
	}
	if payload.Operation != "multiply" {
		t.Fatalf("expected operation 'multiply', got %q", payload.Operation)
	}
}

func TestHandleQueryParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/math?a=10&b=5&op=divide", nil)
	rec := httptest.NewRecorder()

	Handle(context.Background(), rec, req)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var payload mathResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Result != 2 {
		t.Fatalf("expected result 2, got %v", payload.Result)
	}
}

func TestHandleInvalidInput(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.com/math", strings.NewReader(`{"a":1,"op":"add"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	Handle(context.Background(), rec, req)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", res.StatusCode)
	}

	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error == "" {
		t.Fatalf("expected error message, got empty string")
	}
}

func TestDivideByZero(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/math?a=5&b=0&op=divide", nil)
	rec := httptest.NewRecorder()

	Handle(context.Background(), rec, req)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", res.StatusCode)
	}

	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(payload.Error, "divide by zero") {
		t.Fatalf("expected divide by zero error, got %q", payload.Error)
	}
}

func TestHandleEnvelopeInput(t *testing.T) {
	body := `{"input":{"a":2,"b":3,"op":"power"}}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/math", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	Handle(context.Background(), rec, req)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var payload mathResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Operation != "power" {
		t.Fatalf("expected operation power, got %q", payload.Operation)
	}
	if payload.Result != 8 {
		t.Fatalf("expected result 8, got %v", payload.Result)
	}
}
