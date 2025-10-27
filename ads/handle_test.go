package function

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandle ensures that Handle executes without error and returns the
// HTTP 200 status code indicating no errors, and the body "Ash".
func TestHandle(t *testing.T) {
	var (
		w   = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "http://example.com/test", nil)
		res *http.Response
	)

	Handle(context.Background(), w, req)
	res = w.Result()
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("unexpected response code: %v", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != "Ash" {
		t.Fatalf("unexpected response body. got %q, want %q", string(body), "Ash")
	}
}
