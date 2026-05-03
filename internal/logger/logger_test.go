package logger

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetLoggerForTest(t *testing.T) {
	t.Helper()
	std.mu.Lock()
	if std.file != nil {
		_ = std.file.Close()
	}
	std.level = INFO
	std.output = io.Discard
	std.file = nil
	std.mu.Unlock()
}

func readPipeOutput(t *testing.T, reader *os.File, writer *os.File) string {
	t.Helper()
	if err := writer.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	b, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(b)
}

func latestLogFile(t *testing.T, home string) string {
	t.Helper()
	logDir := filepath.Join(home, ".5hAgent", logDirName)
	var newest string
	err := filepath.WalkDir(logDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), logFilePrefix) {
			newest = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk log dir: %v", err)
	}
	if newest == "" {
		t.Fatalf("no log file found under %s", logDir)
	}
	return newest
}

func TestLoggerIsSilentBeforeInit(t *testing.T) {
	resetLoggerForTest(t)
	oldStderr := os.Stderr
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = stderrWriter
	defer func() {
		os.Stderr = oldStderr
		resetLoggerForTest(t)
	}()

	ErrorTag("TOOL", "visible nowhere")
	if got := readPipeOutput(t, stderrReader, stderrWriter); got != "" {
		t.Fatalf("stderr output = %q, want empty", got)
	}
}

func TestInitLogWritesInfoAndErrorOnlyToFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	resetLoggerForTest(t)

	oldStderr := os.Stderr
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = stderrWriter
	defer func() {
		os.Stderr = oldStderr
		resetLoggerForTest(t)
	}()

	logFile, err := InitLog()
	if err != nil {
		t.Fatalf("InitLog() error = %v", err)
	}

	DebugTag("LLM", "debug hidden")
	InfoTag("SYS", "started")
	ErrorTag("TOOL", "failed")
	if got := readPipeOutput(t, stderrReader, stderrWriter); got != "" {
		t.Fatalf("stderr output = %q, want empty", got)
	}

	logBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logFile, err)
	}
	content := string(logBytes)
	if strings.Contains(content, "debug hidden") {
		t.Fatalf("log = %q, did not expect DEBUG message at INFO level", content)
	}
	if !strings.Contains(content, "[INFO][SYS") || !strings.Contains(content, "started") {
		t.Fatalf("log = %q, want SYS info message", content)
	}
	if !strings.Contains(content, "[ERROR][TOOL") || !strings.Contains(content, "failed") {
		t.Fatalf("log = %q, want TOOL error message", content)
	}
}

func TestDebugLevelWritesDebugOnlyToFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	resetLoggerForTest(t)

	oldStderr := os.Stderr
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = stderrWriter
	defer func() {
		os.Stderr = oldStderr
		resetLoggerForTest(t)
	}()

	SetLevel(DEBUG)
	if _, err := InitLog(); err != nil {
		t.Fatalf("InitLog() error = %v", err)
	}

	DebugTag("LLM", "Start: messages=%d", 17)
	if got := readPipeOutput(t, stderrReader, stderrWriter); got != "" {
		t.Fatalf("stderr output = %q, want empty", got)
	}

	logBytes, err := os.ReadFile(latestLogFile(t, tmpHome))
	if err != nil {
		t.Fatalf("read latest log: %v", err)
	}
	if !strings.Contains(string(logBytes), "[DEBUG][LLM") || !strings.Contains(string(logBytes), "Start: messages=17") {
		t.Fatalf("debug log = %q, want LLM debug message", string(logBytes))
	}
}
