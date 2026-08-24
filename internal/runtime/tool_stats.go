package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Ozqi/walle/internal/logger"
	"github.com/Ozqi/walle/internal/toolevent"
	"github.com/Ozqi/walle/internal/utils"
)

type toolFailureStat struct {
	Time             string `json:"time"`
	SessionID        string `json:"session_id,omitempty"`
	Model            string `json:"model,omitempty"`
	ToolName         string `json:"tool_name"`
	ArgumentsSummary string `json:"arguments_summary,omitempty"`
	Error            string `json:"error"`
	Workspace        string `json:"workspace,omitempty"`
	ProcessID        string `json:"process_id,omitempty"`
}

// RecordToolEvent 记录用户会话中的工具失败事件，方便后续诊断。
func (r *Runtime) RecordToolEvent(event toolevent.ToolEvent) {
	r.handleToolEvent(event, "")
}

func (r *Runtime) recordToolFailure(event toolevent.ToolEvent, processID string) {
	if event.Kind != "error" {
		return
	}
	stat := toolFailureStat{
		Time:             time.Now().UTC().Format(time.RFC3339),
		SessionID:        r.SessionID,
		Model:            r.ModelName,
		ToolName:         event.Name,
		ArgumentsSummary: logger.TruncateString(event.Args, 240),
		Error:            logger.TruncateString(event.Error, 500),
		Workspace:        r.ProjectDir,
		ProcessID:        processID,
	}
	if stat.Error == "" {
		stat.Error = logger.TruncateString(event.Text, 500)
	}
	if err := writeToolFailureStats(r, stat); err != nil {
		logger.WarnTag("TOOL", "record tool failure: %v", err)
	}
}

func writeToolFailureStats(r *Runtime, stat toolFailureStat) error {
	// 1. 单条记录编码为一行，避免错误文本破坏 JSONL 边界。
	data, err := json.Marshal(stat)
	if err != nil {
		return fmt.Errorf("marshal failure stat: %w", err)
	}
	line := append(data, '\n')

	// 2. 用户级日志始终写入；项目级日志仅在 Runtime 信息完整时追加。
	configDir, err := utils.GetConfigDir()
	if err != nil {
		return err
	}
	if err := appendToolStat(filepath.Join(configDir, "tool-stats", "failures.jsonl"), line); err != nil {
		return err
	}
	if r != nil && r.ProjectDir != "" && r.Agent != nil {
		projectPath := filepath.Join(projectDataDir(r.ProjectDir), "agents", safeName(r.Agent.Name()), "logs", "tool-failures.jsonl")
		if err := appendToolStat(projectPath, line); err != nil {
			return err
		}
	}
	return nil
}

func appendToolStat(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create tool stats dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open tool stats: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("write tool stats: %w", err)
	}
	return nil
}
