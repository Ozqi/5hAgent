package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "main.md")
	if err := os.WriteFile(path, []byte("\nhello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := Load(tmpDir, "main")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("Load() = %q, want hello", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(t.TempDir(), "missing"); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
