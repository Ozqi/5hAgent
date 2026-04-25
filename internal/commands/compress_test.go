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

type fakeCompressCommandModel struct{}

func (m *fakeCompressCommandModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{Content: "---\n目标: command compress\n进度: - ok\n发现: 无\n状态: compressed\n待处理: 无\n---"}, nil
}

func (m *fakeCompressCommandModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *fakeCompressCommandModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestHandleCompressContext(t *testing.T) {
	mgr := agentctx.NewManager()
	msgCtx, err := mgr.CreateContext()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		if err := mgr.AddMessage(msgCtx, &schema.Message{Role: schema.User, Content: strings.Repeat("m", 10)}); err != nil {
			t.Fatal(err)
		}
	}

	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "compress.md"), []byte("compress now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	compactRoot := filepath.Join(t.TempDir(), "compact")

	result, err := HandleCompress(context.Background(), "/compress context", mgr, msgCtx, &fakeCompressCommandModel{}, promptDir, compactRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Compressed context") {
		t.Fatalf("expected result summary, got %q", result)
	}
	if !strings.Contains(result, "compact/messages") {
		t.Fatalf("expected compact path in result, got %q", result)
	}
}

func TestHandleCompressRequiresContextObjects(t *testing.T) {
	_, err := HandleCompress(context.Background(), "/compress context", nil, nil, &fakeCompressCommandModel{}, "prompt", "compact")
	if err == nil {
		t.Fatal("expected missing context error")
	}
}
