package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContextWindowReadsLoadedOllamaModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			t.Fatalf("path = %q, want /api/ps", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"ornith:9b","context_length":32768}]}`))
	}))
	defer server.Close()

	client := &LLMClient{config: &Config{
		Supplier: "ollama",
		BaseURL:  server.URL + "/v1",
		Model:    "ornith:9b",
	}}
	if got := client.ContextWindow(context.Background()); got != 32768 {
		t.Fatalf("ContextWindow() = %d, want 32768", got)
	}
}

func TestContextWindowSkipsNonOllamaProvider(t *testing.T) {
	client := &LLMClient{config: &Config{Supplier: "mira", BaseURL: "http://127.0.0.1:8787/v1", Model: "gpt"}}
	if got := client.ContextWindow(context.Background()); got != 0 {
		t.Fatalf("ContextWindow() = %d, want 0", got)
	}
}
