package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lzq/5hAgent/internal/logger"
)

func TestHookManagerRunsMatchingToolHook(t *testing.T) {
	projectDir := t.TempDir()
	dataDir := filepath.Join(projectDir, ".5hagent")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	outputPath := filepath.Join(projectDir, "hook-output.jsonl")
	scriptPath := filepath.Join(projectDir, "hook.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\ncat >> \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	config := hookConfigFile{Hooks: []hookSpec{{
		Event:   "tool_error",
		Command: []string{scriptPath, outputPath},
		Timeout: "1s",
	}}}
	configData, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "hooks.json"), configData, 0o644); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}

	manager := loadHookManager(projectDir, "session-a")
	manager.Run(logger.ToolEvent{Kind: "result", Name: "base.read_file"})
	manager.Run(logger.ToolEvent{Kind: "error", Name: "base.read_file", Args: `{"path":"x"}`, Error: "missing"})

	var data []byte
	for i := 0; i < 50; i++ {
		data, err = os.ReadFile(outputPath)
		if err == nil && len(data) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("hook output lines = %d, want 1: %s", len(lines), string(data))
	}
	var payload hookPayload
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("unmarshal hook payload: %v", err)
	}
	if payload.Event != "tool_error" || payload.Tool != "base.read_file" || payload.SessionID != "session-a" || payload.Workspace != projectDir {
		t.Fatalf("payload = %+v", payload)
	}
}
