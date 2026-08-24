// Package commands 解析 TUI 和 daemon 交互会话中的内置 slash command。
package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	agentctx "github.com/Ozqi/walle/internal/context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// HandleCompress 校验命令后调用上下文管理器压缩消息，并返回压缩结果和当前消息快照。
// 副作用：可能写入消息归档及 session 存储；compactRoot 为空时使用相对目录 compact。
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
