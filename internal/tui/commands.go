package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ozqi/walle/internal/commands"
	"github.com/Ozqi/walle/internal/toolevent"
	"github.com/Ozqi/walle/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

// submit 将输入提交给 attached daemon 或本地 Agent。
// attached 模式只在本地截获 /detach 和 /stop；本地模式同步处理 slash command，普通输入异步运行 Agent。
func (m *AppModel) submit() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	m.lastInput = text
	if m.remoteSubmit != nil {
		m.input.Reset()
		if text == "/detach" {
			return tea.Quit
		}
		if text == "/stop" {
			if m.remoteStop == nil {
				m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: "/stop is not available"})
				m.currentStatus = "error"
				m.refreshView()
				return nil
			}
			if err := m.remoteStop(); err != nil {
				m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: err.Error()})
				m.currentStatus = "error"
				m.refreshView()
				return nil
			}
			m.currentStatus = "stopping"
			m.refreshView()
			return nil
		}
		if m.busy {
			m.currentStatus = "busy"
			m.refreshView()
			return nil
		}
		if err := m.remoteSubmit(text); err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: err.Error()})
			m.currentStatus = "error"
			m.refreshView()
			return nil
		}
		if !strings.HasPrefix(text, "/") {
			m.busy = true
		}
		m.currentStatus = "submitted"
		m.refreshView()
		return tickSpinner()
	}
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
		event := toolevent.ToolEvent{Kind: "call", Name: "base.exec_shell", Args: `{"cmd":"sleep 5 && echo debug-done"}`}
		m.toolCalls++
		m.lastTool = fallback(tools.DisplayName(event.Name), event.Name)
		m.applyToolEvent(event)
		m.currentStatus = "debug tool running"
		m.refreshView()
		return tea.Batch(tickSpinner(), func() tea.Msg {
			time.Sleep(5 * time.Second)
			return debugToolResultMsg{event: toolevent.ToolEvent{Kind: "result", Name: "base.exec_shell", Args: event.Args, Text: "  ⎿ debug-done\n"}}
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

// handleStopCommand 取消本地 Agent run，并立即把 UI 状态切回 stopped。
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

// runAgent 在后台执行本地 Agent，并把 token、reasoning 和终态送回 Bubble Tea 事件循环。
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
			sb.WriteString(fmt.Sprintf("  %s%s - %s (updated: %s)\n", marker, s.ID, strings.ReplaceAll(s.Title, "\uFFFD", ""), s.UpdatedAt.Format("2006-01-02 15:04")))
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
