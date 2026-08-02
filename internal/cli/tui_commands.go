// tui_commands.go - TUI 输入和 slash 命令处理
// 功能：处理用户提交、内置 slash 命令、模型切换、任务运行和停止。
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lzq/5hAgent/internal/commands"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/tools"
)

func (m *AppModel) submit() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	m.lastInput = text
	fields := strings.Fields(text)
	cmdName := ""
	if len(fields) > 0 {
		cmdName = fields[0]
	}
	if cmdName == "/stop" {
		return m.handleStopCommand(text)
	}
	if m.busy {
		m.currentStatus = "busy"
		return nil
	}

	m.input.Reset()
	m.entries = append(m.entries, conversationEntry{Role: roleUser, Content: text})
	m.refreshView()

	if cmdName == "/debug" && os.Getenv("5HAGENT_TUI_DEBUG") == "1" {
		return m.handleDebugCommand(text)
	}

	if cmdName == "/skill" {
		result, err := commands.HandleSkill(text, m.skillMgr)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if cmdName == "/task" {
		result, err := commands.HandleTask(text, m.taskList)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if cmdName == "/compress" {
		result, err := commands.HandleCompress(m.ctx, text, m.ctxManager, m.messageCtx, m.ag.GetModel(), m.promptDir, "compact")
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if cmdName == "/mcp" {
		result, err := commands.HandleMCP(text)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if cmdName == "/session" {
		result, err := m.handleSessionCommand(text)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if cmdName == "/model" {
		return m.handleModelCommand(text)
	}

	if cmdName == "/run" {
		return m.handleRunCommand(text)
	}

	if strings.HasPrefix(text, "/") {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: "unknown slash command"})
		m.refreshView()
		return nil
	}

	m.busy = true
	m.currentStatus = "thinking"
	m.currentAssistant = -1
	runCtx, cancel := context.WithCancel(m.ctx)
	m.runCancel = cancel
	m.refreshView()

	go m.runAgent(runCtx, cancel, text)
	return tickSpinner()
}

// handleDebugCommand 仅用于本地 TUI 样式自查，默认不启用。
// 调用层级：submit -> handleDebugCommand -> applyToolEvent/refreshView。
// 主要步骤：注入可控 tool running 事件，再延迟注入 result，方便 tmux 捕获中间态。
func (m *AppModel) handleDebugCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	if len(fields) >= 2 && fields[1] == "tool-running" {
		event := logger.ToolEvent{Kind: "call", Name: "base.exec_shell", Args: `{"cmd":"sleep 5 && echo debug-done"}`}
		m.toolCalls++
		m.lastTool = fallback(tools.DisplayName(event.Name), event.Name)
		m.applyToolEvent(event)
		m.currentStatus = "debug tool running"
		m.refreshView()
		return tea.Batch(tickSpinner(), func() tea.Msg {
			time.Sleep(5 * time.Second)
			return debugToolResultMsg{event: logger.ToolEvent{Kind: "result", Name: "base.exec_shell", Args: event.Args, Text: "  ⎿ debug-done\n"}}
		})
	}
	m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: "unknown debug command"})
	m.refreshView()
	return nil
}

// handleModelCommand 切换当前 TUI runtime 使用的模型。
// 调用层级：submit -> handleModelCommand -> runtime.SwitchModel。
// 主要步骤：解析 provider/model；调用 runtime 回调重建并绑定模型；更新状态栏 modelName。
func (m *AppModel) handleModelCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	if len(fields) == 1 {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: modelUsage()})
		m.currentStatus = "idle"
		m.refreshView()
		return nil
	}
	if len(fields) != 2 {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: modelUsage()})
		m.currentStatus = "idle"
		m.refreshView()
		return nil
	}
	if m.switchModel == nil {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: "/model is not available in this runtime"})
		m.refreshView()
		return nil
	}
	modelName, err := m.switchModel(m.ctx, fields[1])
	if err != nil {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		m.currentStatus = "error"
		m.refreshView()
		return nil
	}
	m.modelName = modelName
	m.currentStatus = "idle"
	m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: "Switched model: " + modelName})
	m.refreshView()
	return nil
}

func modelUsage() string {
	var sb strings.Builder
	sb.WriteString("usage: /model <provider/model>\n\nAvailable examples:\n")
	for _, ref := range modelHints {
		sb.WriteString("  ")
		sb.WriteString(ref)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

func cleanDisplayText(text string) string {
	return strings.ReplaceAll(text, "\uFFFD", "")
}

// handleRunCommand 启动 task.md 连续执行模式。
// 调用层级：submit -> handleRunCommand -> runtime.RunTasksUntilDone。
// 主要步骤：校验命令参数；标记 TUI busy；后台执行 runtime 回调；完成后显示汇总。
func (m *AppModel) handleRunCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	if len(fields) > 1 {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: "usage: /run"})
		m.refreshView()
		return nil
	}
	if m.runTasks == nil {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: "/run is not available in this runtime"})
		m.refreshView()
		return nil
	}
	m.busy = true
	m.currentStatus = "running tasks"
	m.currentAssistant = -1
	m.runLines = nil
	m.runEntry = len(m.entries)
	m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: "/run", Content: "running..."})
	m.refreshView()
	return tea.Batch(tickSpinner(), func() tea.Msg {
		summary, err := m.runTasks(m.ctx, func(event logger.ToolEvent) {
			if m.program != nil {
				m.program.Send(toolEventMsg{event: event})
			}
		})
		return runTasksDoneMsg{summary: summary, err: err}
	})
}

func (m *AppModel) handleStopCommand(text string) tea.Cmd {
	m.input.Reset()
	if !m.busy || m.runCancel == nil {
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: "no active run"})
		m.refreshView()
		return nil
	}
	m.runCancel()
	m.runCancel = nil
	m.busy = false
	m.currentAssistant = -1
	m.currentStatus = "stopped"
	m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: "stopped current run"})
	m.refreshView()
	return nil
}

func (m *AppModel) runAgent(runCtx context.Context, cancel context.CancelFunc, input string) {
	if m.program == nil {
		return
	}
	defer func() {
		m.runCancel = nil
		cancel()
	}()
	_, err := m.ag.RunStream(runCtx, m.messageCtx, input, func(token string) {
		m.program.Send(assistantTokenMsg{token: token})
	}, func(token string) {
		m.program.Send(assistantThinkingMsg{token: token})
	})
	if err != nil {
		if runCtx.Err() != nil {
			return
		}
		m.program.Send(assistantErrorMsg{err: err})
		return
	}
	m.program.Send(assistantDoneMsg{})
}

// handleSessionCommand 处理 /session 命令
// /session list - 列出所有会话
// /session new - 创建新会话
// /session <id> - 切换到指定会话
func (m *AppModel) handleSessionCommand(text string) (string, error) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return fmt.Sprintf("Current session: %s (%s)", m.sessionID, m.ctxManager.GetSessionTitle(m.messageCtx)), nil
	}

	cmd := parts[1]
	switch cmd {
	case "list", "ls":
		sessions, err := m.ctxManager.ListSessions()
		if err != nil {
			return "", err
		}
		if len(sessions) == 0 {
			return "No sessions found", nil
		}
		var sb strings.Builder
		sb.WriteString("Sessions:\n")
		for _, s := range sessions {
			marker := "  "
			if s.ID == m.sessionID {
				marker = "→ "
			}
			sb.WriteString(fmt.Sprintf("  %s%s - %s (updated: %s)\n", marker, s.ID, cleanDisplayText(s.Title), s.UpdatedAt.Format("2006-01-02 15:04")))
		}
		return sb.String(), nil
	case "new":
		newCtx, err := m.ctxManager.CreateContext("")
		if err != nil {
			return "", err
		}
		m.messageCtx = newCtx
		m.sessionID = m.ctxManager.GetSessionID(newCtx)
		m.entries = nil
		m.refreshView()
		return fmt.Sprintf("Created new session: %s", m.sessionID), nil
	default:
		newCtx, err := m.ctxManager.SwitchSession(m.messageCtx, cmd)
		if err != nil {
			return "", err
		}
		m.messageCtx = newCtx
		m.sessionID = cmd
		m.entries = nil
		m.refreshView()
		return fmt.Sprintf("Switched to session: %s (%s)", m.sessionID, m.ctxManager.GetSessionTitle(newCtx)), nil
	}
}
