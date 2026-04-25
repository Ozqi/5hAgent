package toolmeta_test

import (
	"testing"

	"github.com/lzq/5hAgent/internal/toolmeta"
	"github.com/lzq/5hAgent/internal/tools"
)

func TestRegistryPublishesToolMetadata(t *testing.T) {
	toolmeta.Reset()
	if err := tools.InitRegistry(nil, nil); err != nil {
		t.Fatalf("failed to initialize tool registry: %v", err)
	}

	meta, ok := toolmeta.Lookup("base.read_file")
	if !ok {
		t.Fatal("expected metadata for base.read_file")
	}
	if meta.Category != toolmeta.CategoryBase {
		t.Fatalf("expected base category, got %q", meta.Category)
	}
	if meta.DisplayName != "read_file" {
		t.Fatalf("expected display name %q, got %q", "read_file", meta.DisplayName)
	}
	if meta.OriginalName != "read_file" {
		t.Fatalf("expected original name %q, got %q", "read_file", meta.OriginalName)
	}

	taskMeta, ok := toolmeta.Lookup("task.task")
	if ok {
		t.Fatalf("did not expect task metadata without task tool registration, got %+v", taskMeta)
	}
}
