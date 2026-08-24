package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Ozqi/walle/internal/logger"
	"github.com/Ozqi/walle/internal/toolevent"
)

const defaultHookTimeout = 5 * time.Second

type hookConfigFile struct {
	Hooks []hookSpec `json:"hooks"`
}

type hookSpec struct {
	Event   string   `json:"event"`
	Command []string `json:"command"`
	Timeout string   `json:"timeout,omitempty"`
}

// HookManager 保存项目 hook 配置，并按工具事件异步执行匹配命令。
type HookManager struct {
	workspace string
	sessionID string
	hooks     []hookSpec
}

type hookPayload struct {
	Event         string `json:"event"`
	Tool          string `json:"tool,omitempty"`
	ArgsSummary   string `json:"args_summary,omitempty"`
	ResultSummary string `json:"result_summary,omitempty"`
	Error         string `json:"error,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Time          string `json:"time"`
}

func loadHookManager(projectDir string, sessionID string) *HookManager {
	manager := &HookManager{workspace: projectDir, sessionID: sessionID}
	path := filepath.Join(projectDataDir(projectDir), "hooks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return manager
	}
	var cfg hookConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		logger.WarnTag("HOOK", "load hooks %s: %v", path, err)
		return manager
	}
	for _, hook := range cfg.Hooks {
		if hook.Event == "" || len(hook.Command) == 0 {
			continue
		}
		manager.hooks = append(manager.hooks, hook)
	}
	return manager
}

// Run 根据工具事件类型异步触发所有匹配 hook；命令失败只记录日志。
func (m *HookManager) Run(event toolevent.ToolEvent) {
	if m == nil || len(m.hooks) == 0 {
		return
	}
	name := hookEventName(event.Kind)
	if name == "" {
		return
	}
	payload := hookPayload{
		Event:         name,
		Tool:          event.Name,
		ArgsSummary:   logger.TruncateString(event.Args, 240),
		ResultSummary: logger.TruncateString(event.Result, 500),
		Error:         logger.TruncateString(event.Error, 500),
		Workspace:     m.workspace,
		SessionID:     m.sessionID,
		Time:          time.Now().UTC().Format(time.RFC3339),
	}
	// 每个匹配 hook 独立执行；调用方不等待结果，失败只写日志。
	for _, hook := range m.hooks {
		if hook.Event != name {
			continue
		}
		spec := hook
		go m.runHook(spec, payload)
	}
}

func (m *HookManager) runHook(spec hookSpec, payload hookPayload) {
	// 1. 解析单 hook 超时；无效值回退到默认上限，避免命令无限驻留。
	timeout := defaultHookTimeout
	if spec.Timeout != "" {
		if parsed, err := time.ParseDuration(spec.Timeout); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	data, err := json.Marshal(payload)
	if err != nil {
		logger.WarnTag("HOOK", "marshal hook payload: %v", err)
		return
	}
	// 2. 在项目工作区运行命令，并通过 stdin 发送单行 JSON；不继承输出流。
	cmd := exec.CommandContext(ctx, spec.Command[0], spec.Command[1:]...)
	cmd.Dir = m.workspace
	cmd.Stdin = bytes.NewReader(append(data, '\n'))
	if err := cmd.Run(); err != nil {
		logger.WarnTag("HOOK", "run hook %s: %v", fmt.Sprint(spec.Command), err)
	}
}

func hookEventName(kind string) string {
	switch kind {
	case "call":
		return "tool_start"
	case "result":
		return "tool_end"
	case "error":
		return "tool_error"
	default:
		return ""
	}
}
