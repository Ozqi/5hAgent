package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintErrorWritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = stderrWriter
	defer func() { os.Stderr = oldStderr }()

	PrintError(errors.New("boom"))
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	b, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if !strings.Contains(string(b), "Error: boom") {
		t.Fatalf("stderr = %q, want error text", string(b))
	}
}
