package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOpenRouterCompleteSendsCompletionRequest(t *testing.T) {
	provider := &OpenRouter{
		apiKey:   "test-key",
		endpoint: "https://example.test/chat/completions",
		httpClient: &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost {
				t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
			}
			if authorization := request.Header.Get("Authorization"); authorization != "Bearer test-key" {
				t.Errorf("Authorization = %q, want %q", authorization, "Bearer test-key")
			}
			if contentType := request.Header.Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			var completion CompletionRequest
			if err := json.NewDecoder(request.Body).Decode(&completion); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			if completion.Model != "test-model" {
				t.Errorf("model = %q, want %q", completion.Model, "test-model")
			}
			if len(completion.Messages) != 1 || completion.Messages[0] != (Message{Role: "user", Content: "hello"}) {
				t.Errorf("messages = %#v, want one user message", completion.Messages)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"hello back"}}]}`)),
				Header:     make(http.Header),
			}, nil
		})},
	}

	answer, err := provider.Complete(context.Background(), CompletionRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if answer != "hello back" {
		t.Errorf("Complete() answer = %q, want %q", answer, "hello back")
	}
}

func TestOpenRouterCompleteReturnsErrorForNonSuccessResponse(t *testing.T) {
	provider := &OpenRouter{
		endpoint: "https://example.test/chat/completions",
		httpClient: &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"invalid key"}}`)),
				Header:     make(http.Header),
			}, nil
		})},
	}

	_, err := provider.Complete(context.Background(), CompletionRequest{})
	if err == nil || !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Errorf("Complete() error = %v, want an error containing %q", err, "401 Unauthorized")
	}
}
