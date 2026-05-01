package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

type fakeCompressModel struct{}

func (m *fakeCompressModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{Content: "summary"}, nil
}

func (m *fakeCompressModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *fakeCompressModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestHandleCompressCompressesContext(t *testing.T) {
	mgr := agentctx.NewManager()
	msgCtx, err := mgr.CreateContext("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		if err := mgr.AddMessage(msgCtx, &schema.Message{Role: schema.User, Content: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "compress.md"), []byte("compress please\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := HandleCompress(context.Background(), "/compress", mgr, msgCtx, &fakeCompressModel{}, promptDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Compressed context:") {
		t.Fatalf("expected compression result, got %q", result)
	}
	if !strings.Contains(result, "Archive:") {
		t.Fatalf("expected archive path, got %q", result)
	}
	if !strings.Contains(result, "Compressed context messages:") {
		t.Fatalf("expected compressed context dump, got %q", result)
	}
	if !strings.Contains(result, "[system] [对话历史摘要]\nsummary") {
		t.Fatalf("expected summary message in result, got %q", result)
	}
}

func TestHandleCompressRejectsExtraArgs(t *testing.T) {
	mgr := agentctx.NewManager()
	msgCtx, err := mgr.CreateContext("")
	if err != nil {
		t.Fatal(err)
	}

	_, err = HandleCompress(context.Background(), "/compress now", mgr, msgCtx, &fakeCompressModel{}, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "usage: /compress") {
		t.Fatalf("unexpected error: %v", err)
	}
}
