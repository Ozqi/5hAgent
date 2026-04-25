package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/model"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

func HandleCompress(goCtx context.Context, cmd string, mgr *agentctx.Manager, msgCtx *agentctx.Context, llm model.ToolCallingChatModel, promptDir string, compactRoot string) (string, error) {
	if mgr == nil || msgCtx == nil {
		return "", fmt.Errorf("context manager and message context are required")
	}
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return "", fmt.Errorf("usage: /compress <context|task|tool>")
	}

	switch parts[1] {
	case "context":
		if compactRoot == "" {
			compactRoot = "compact"
		}
		archiveDir := filepath.Join(compactRoot, "messages")
		result, err := mgr.ManualCompress(goCtx, msgCtx, llm, promptDir, archiveDir)
		if err != nil {
			return "", err
		}
		if result.Before == result.After {
			return fmt.Sprintf("Context not compressed (%d messages)", result.Before), nil
		}
		return fmt.Sprintf("Compressed context: %d -> %d messages\nArchive: %s", result.Before, result.After, result.ArchivePath), nil
	case "task":
		return "", fmt.Errorf("/compress task is not implemented yet")
	case "tool":
		return "", fmt.Errorf("/compress tool is not implemented yet")
	default:
		return "", fmt.Errorf("unknown compress target: %s", parts[1])
	}
}
