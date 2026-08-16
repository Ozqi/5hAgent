package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lzq/5hAgent/internal/logger"
)

func TestRecordToolEventWritesFailureStats(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	rt := &Runtime{
		SessionID:  "session-a",
		ModelName:  "model-a",
		ProjectDir: projectDir,
	}

	rt.RecordToolEvent(logger.ToolEvent{
		Kind:  "error",
		Name:  "base.read_file",
		Args:  `{"path":"/tmp/missing"}`,
		Error: "file not found",
	})

	userPath := filepath.Join(home, ".5hAgent", "tool-stats", "failures.jsonl")
	data, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("ReadFile(user stats) error = %v", err)
	}
	var entry toolFailureStat
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", string(data), err)
	}
	if entry.SessionID != "session-a" || entry.Model != "model-a" || entry.ToolName != "base.read_file" || entry.Error != "file not found" {
		t.Fatalf("entry = %+v, want session/model/tool/error", entry)
	}
	if entry.Workspace != projectDir {
		t.Fatalf("workspace = %q, want %q", entry.Workspace, projectDir)
	}
}

func TestRecordToolEventIgnoresNonErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rt := &Runtime{ProjectDir: t.TempDir()}

	rt.RecordToolEvent(logger.ToolEvent{Kind: "result", Name: "base.read_file"})

	userPath := filepath.Join(home, ".5hAgent", "tool-stats", "failures.jsonl")
	if _, err := os.Stat(userPath); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want not exist", userPath, err)
	}
}
