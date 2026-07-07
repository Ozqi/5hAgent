package mcp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewStdioClientReturnsWhenProcessExits(t *testing.T) {
	start := time.Now()
	_, err := NewStdioClient(context.Background(), StdioClientConfig{
		Name:           "missing",
		Command:        "node",
		Args:           []string{"/path/to/nonexistent/mcp-server.js"},
		StartupTimeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatalf("NewStdioClient() error = nil, want startup failure")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NewStdioClient() error = %v, want context canceled after child process exit", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("NewStdioClient() took %v, want early return before startup timeout", elapsed)
	}
}
