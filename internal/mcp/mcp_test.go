package mcp

import (
	"testing"
	"time"
)

func TestServerConfigValidate(t *testing.T) {
	valid := ServerConfig{Name: "claude_context", Command: "npx", Args: []string{"-y", "server"}, StartupTimeout: 5 * time.Second}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	invalid := ServerConfig{Name: "", Command: "npx"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected missing name error")
	}

	invalid = ServerConfig{Name: "claude context", Command: "npx"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestFullToolName(t *testing.T) {
	got := FullToolName("claude_context", "search_code")
	if got != "mcp.claude_context.search_code" {
		t.Fatalf("unexpected full tool name: %q", got)
	}
}
