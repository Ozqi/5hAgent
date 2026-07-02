// compress.go - /compress 命令处理
// 功能：手动触发当前上下文压缩
// 导出函数：HandleCompress
package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

func HandleCompress(goCtx context.Context, cmd string, mgr *agentctx.Manager, msgCtx *agentctx.Context, llm model.ToolCallingChatModel, promptDir string, compactRoot string) (string, error) {
	if mgr == nil || msgCtx == nil {
		return "", fmt.Errorf("context manager and message context are required")
	}
	parts := strings.Fields(cmd)
	if len(parts) != 1 || parts[0] != "/compress" {
		return "", fmt.Errorf("usage: /compress")
	}

	if compactRoot == "" {
		compactRoot = "compact"
	}
	archiveDir := filepath.Join(compactRoot, "messages")
	result, err := mgr.ManualCompress(goCtx, msgCtx, llm, promptDir, archiveDir)
	if err != nil {
		return "", err
	}
	messages, err := mgr.GetMessages(msgCtx)
	if err != nil {
		return "", err
	}
	dump := formatContext(messages)
	if result.Before == result.After {
		return fmt.Sprintf("Context not compressed (%d messages)\n\nCompressed context messages:\n%s", result.Before, dump), nil
	}
	return fmt.Sprintf("Compressed context: %d -> %d messages\nArchive: %s\n\nCompressed context messages:\n%s", result.Before, result.After, result.ArchivePath, dump), nil
}

func formatContext(messages []*schema.Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString("[")
		sb.WriteString(string(msg.Role))
		sb.WriteString("] ")
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
