package runtime

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/task"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String()
}

func TestHeadlessWorkLogPrintsTokensAndToolEvents(t *testing.T) {
	defer logger.SetToolEventSink(nil)
	dataDir := t.TempDir()
	startedAt := time.Date(2026, 6, 23, 10, 11, 12, 0, time.UTC)
	log := newHeadlessWorkLog(true, dataDir, "Demo Agent", startedAt)
	output := captureStdout(t, func() {
		log.Start(&task.Task{ID: "demo", Title: "Demo task"})
		log.OnToken("hello")
		logger.PrintToolCall("base.read_file", `{"path":"README.md"}`, false)
		logger.PrintToolResult("base.read_file", `{"path":"README.md"}`, `{"total_lines":3}`)
		log.OnToken("done")
		log.End(nil)
	})

	for _, want := range []string{
		"task: demo | Demo task",
		"worklog:",
		"assistant:",
		"hello",
		"read_file",
		"path: README.md",
		"total lines",
		"done",
		"status: completed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}

	logPath := filepath.Join(dataDir, "agents", "Demo-Agent", "logs", "20260623-101112-demo.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read work log: %v", err)
	}
	for _, want := range []string{
		"# Headless Work Log",
		"- agent: Demo Agent",
		"- task: demo",
		"- title: Demo task",
		"## Assistant",
		"hello",
		"## Tool Event",
		"read_file",
		"path: README.md",
		"total lines",
		"done",
		"## Status",
		"completed",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("work log = %q, want %q", string(content), want)
		}
	}
}

func TestHeadlessWorkLogQuietSuppressesConsoleButWritesFile(t *testing.T) {
	defer logger.SetToolEventSink(nil)
	dataDir := t.TempDir()
	startedAt := time.Date(2026, 6, 23, 10, 11, 12, 0, time.UTC)
	log := newHeadlessWorkLog(false, dataDir, "Demo Agent", startedAt)
	output := captureStdout(t, func() {
		log.Start(&task.Task{ID: "demo", Title: "Demo task"})
		log.OnToken("hidden")
		logger.PrintToolCall("base.read_file", `{"path":"README.md"}`, false)
		log.End(nil)
	})
	if output != "" {
		t.Fatalf("quiet output = %q, want empty", output)
	}

	logPath := filepath.Join(dataDir, "agents", "Demo-Agent", "logs", "20260623-101112-demo.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read work log: %v", err)
	}
	for _, want := range []string{
		"hidden",
		"read_file",
		"path: README.md",
		"completed",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("work log = %q, want %q", string(content), want)
		}
	}
}
