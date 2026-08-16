// registry_test.go - 验证本地工具注册元数据与实际能力一致。
package tools

import "testing"

func TestReadMDMetadataAllowsWrites(t *testing.T) {
	registry := NewRegistry()
	registry.SetWorkspaceRoot(t.TempDir())
	if err := registry.Init(nil, nil); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	meta, ok := registry.lookupMeta(readMDToolName)
	if !ok {
		t.Fatalf("metadata for %q not found", readMDToolName)
	}
	if meta.ReadOnly {
		t.Fatalf("metadata for %q is read-only despite replace/delete actions", readMDToolName)
	}
}
