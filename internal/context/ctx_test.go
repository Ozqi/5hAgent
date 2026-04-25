package context

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeCompressModel struct{}

func (m *fakeCompressModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{Content: "---\n目标: keep working\n进度: - compressed\n发现: 无\n状态: compressed\n待处理: 无\n---"}, nil
}

func (m *fakeCompressModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *fakeCompressModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestManualCompressArchivesMessages(t *testing.T) {
	mgr := NewManager()
	msgCtx, err := mgr.CreateContext()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		if err := mgr.AddMessage(msgCtx, &schema.Message{Role: schema.User, Content: strings.Repeat("x", 8)}); err != nil {
			t.Fatal(err)
		}
	}

	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "compress.md"), []byte("compress please\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(t.TempDir(), "compact", "messages")

	result, err := mgr.ManualCompress(context.Background(), msgCtx, &fakeCompressModel{}, promptDir, archiveDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Before <= result.After {
		t.Fatalf("expected compression to reduce messages, before=%d after=%d", result.Before, result.After)
	}
	if result.ArchivePath == "" {
		t.Fatal("expected archive path")
	}
	data, err := os.ReadFile(result.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# Compressed Context Archive") {
		t.Fatalf("expected archive header, got %q", text)
	}
	if !strings.Contains(text, "目标: keep working") {
		t.Fatalf("expected summary in archive, got %q", text)
	}

	messages, err := mgr.GetMessages(msgCtx)
	if err != nil {
		t.Fatal(err)
	}
	var foundSummary bool
	for _, msg := range messages {
		if strings.Contains(msg.Content, "[对话历史摘要]") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatal("expected summary message in context")
	}
}
