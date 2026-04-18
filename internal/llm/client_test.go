package llm

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	config := &Config{
		APIKey:    "test-api-key",
		BaseURL:   "https://test.api.com",
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
	}

	ctx := context.Background()
	client, err := NewClient(ctx, config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client.GetConfig().APIKey != config.APIKey {
		t.Errorf("Expected APIKey '%s', got '%s'", config.APIKey, client.GetConfig().APIKey)
	}

	if client.GetModel() == nil {
		t.Error("Expected non-nil model")
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	ctx := context.Background()
	_, err := NewClient(ctx, nil)
	if err == nil {
		t.Error("Expected error when config is nil, got nil")
	}
}

func TestNewClient_EmptyAPIKey(t *testing.T) {
	config := &Config{
		BaseURL: "https://test.api.com",
		Model:   "claude-sonnet-4-6",
	}

	ctx := context.Background()
	_, err := NewClient(ctx, config)
	if err == nil {
		t.Error("Expected error when API key is empty, got nil")
	}
}
