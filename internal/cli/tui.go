// tui.go - 终端 UI 主界面
// 功能：Bubble Tea 构建的交互式对话界面，显示对话/状态面板，支持 /task /skill /compress 命令
// 主要类型：AppModel, conversationEntry, statusSnapshot
// 导出函数：NewAppModel, LaunchTUI
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ansi "github.com/charmbracelet/x/ansi"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/commands"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/tools"
)

const (
	roleUser         = "user"
	roleAssistant    = "assistant"
	roleTool         = "tool"
	roleSystem       = "system"
	roleHint         = "hint"
	roleThinking     = "thinking"
	defaultTUIWidth  = 100
	defaultTUIHeight = 30
)

type conversationEntry struct {
	Role        string
	Content     string
	ToolName    string
	ToolArgs    string
	ToolKey     string
	ToolState   string
	ToolOutput  string
	ToolOpen    bool
	SystemTitle string
}

type statusSnapshot struct {
	Runtime             runtimeMeta
	EnabledSkills       []string
	HighlightedTaskLine []string
}

type runtimeMeta struct {
	Busy             bool
	State            string
	Turn             int
	TokenUsed        int
	TokenLimit       int
	ScrollPercent    int
	ContextMessages  int
	ContextSummaries int
	ToolCallsTotal   int
	LastToolName     string
	SessionID        string
	Workdir          string
	Git              gitMeta
	ActiveTasks      int
	TotalTasks       int
}

type gitMeta struct {
	Repo      bool
	Worktree  bool
	Branch    string
	Dirty     bool
	Shortstat string
}

// AppModel 保存 TUI 当前帧所需的全部状态。
// 调用层级：LaunchTUI -> NewAppModel -> Bubble Tea Update/View。
// 设计边界：UI 层只持有 runtime 对象引用和渲染快照，不在 View 中直接拼业务查询逻辑。
type AppModel struct {
	program     *tea.Program
	ag          *agent.Agent
	modelName   string
	agentName   string
	sessionID   string
	promptDir   string
	taskList    *task.TaskList
	skillMgr    *skill.Manager
	ctxManager  *agentctx.Manager
	messageCtx  *agentctx.Context
	runTasks    RunTasksFunc
	switchModel SwitchModelFunc
	ctx         context.Context

	width  int
	height int
	busy   bool

	viewport  viewport.Model
	input     textarea.Model
	entries   []conversationEntry
	viewText  string
	toolCalls int
	lastTool  string

	currentAssistant int
	currentStatus    string
	spinnerFrame     int
	escPending       bool
	lastEscAt        time.Time
	autoScroll       bool
	metaCache        cachedMeta
}

type cachedMeta struct {
	Workdir  string
	Git      gitMeta
	LoadedAt time.Time
}

type assistantTokenMsg struct {
	token string
}

type assistantThinkingMsg struct {
	token string
}

type assistantDoneMsg struct{}

type assistantErrorMsg struct {
	err error
}

type toolEventMsg struct {
	event logger.ToolEvent
}

type spinnerTickMsg struct{}

type debugToolResultMsg struct {
	event logger.ToolEvent
}

type runTasksDoneMsg struct {
	summary string
	err     error
}

// RunTasksFunc 是 TUI /run 命令调用 runtime 连续执行 task.md 的薄接口。
type RunTasksFunc func(context.Context, func(logger.ToolEvent)) (string, error)

// SwitchModelFunc 是 TUI /model 命令切换当前 Runtime 模型的薄接口。
type SwitchModelFunc func(context.Context, string) (string, error)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var launchMu sync.Mutex

var (
	colorBg      = lipgloss.Color("#12131d")
	colorBlack   = lipgloss.Color("#000000")
	colorSurface = lipgloss.Color("#1a1b26")
	colorGreen   = lipgloss.Color("#9ece6a")
	colorBlue    = lipgloss.Color("#7aa2f7")
	colorPurple  = lipgloss.Color("#bb9af7")
	colorYellow  = lipgloss.Color("#e0af68")
	colorText    = lipgloss.Color("#a9b1d6")
	colorGray    = lipgloss.Color("#565f89")
	colorWhite   = lipgloss.Color("#e2e1f1")
	colorCommand = lipgloss.Color("#89b4fa")
	colorResult  = lipgloss.Color("#cdd6f4")
	colorError   = lipgloss.Color("#f38ba8")
	colorWarnBg  = lipgloss.Color("#3a252a")
	colorOkBg    = lipgloss.Color("#2a3832")
	colorInputBg = lipgloss.Color("#404a4f")
	colorInputFg = lipgloss.Color("#dce4e3")
	colorMuted   = lipgloss.Color("#93a799")

	mainViewStyle = lipgloss.NewStyle().
			Padding(0, 0)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	inputShellStyle = lipgloss.NewStyle().
			Background(colorInputBg).
			Foreground(colorInputFg).
			Padding(0, 0)

	slashHintStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	messageBoxStyle = lipgloss.NewStyle().
			Padding(0, 2)
)

type slashCommandHint struct {
	Name  string
	Usage string
	Desc  string
}

var slashCommandHints = []slashCommandHint{
	{Name: "/task", Usage: "/task <list|create|update|get|delete|archive|reopen>", Desc: "task file"},
	{Name: "/skill", Usage: "/skill <list|enable|disable|show>", Desc: "skills"},
	{Name: "/compress", Usage: "/compress [compact|truncate]", Desc: "context"},
	{Name: "/mcp", Usage: "/mcp <list|add|remove|enable|disable>", Desc: "mcp servers"},
	{Name: "/session", Usage: "/session <new|list|switch|save|drop>", Desc: "sessions"},
	{Name: "/run", Usage: "/run", Desc: "run task.md until no pending tasks"},
	{Name: "/model", Usage: "/model <provider/model>", Desc: "mira/gpt-5.4, mira/gpt-5.5, mira/glm-5.2"},
}

var modelHints = []string{
	"mira/gpt-5.4",
	"mira/gpt-5.5",
	"mira/glm-5.2",
	"mira/claude-opus-4-6",
}

func NewAppModel(ctx context.Context, ag *agent.Agent, modelName string, promptDir string, taskList *task.TaskList, skillMgr *skill.Manager, ctxManager *agentctx.Manager, messageCtx *agentctx.Context, sessionID string, runTasks RunTasksFunc, switchModel SwitchModelFunc) *AppModel {
	vp := viewport.New(0, 0)
	// viewport 自身支持滚轮，但还需要 LaunchTUI 开启 Bubble Tea mouse mode。
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 2

	input := textarea.New()
	input.Placeholder = ""
	input.Focus()
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.Prompt = "> "
	// 输入框使用参考 tmux 对话窗口的低对比深灰条，避免大白块抢视觉焦点。
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorGreen).Background(colorInputBg).Bold(true)
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorInputFg).Background(colorInputBg)
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorGray).Background(colorInputBg)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(colorInputFg).Background(colorInputBg)
	input.FocusedStyle.CursorLineNumber = lipgloss.NewStyle().Foreground(colorGray).Background(colorInputBg)
	input.BlurredStyle = input.FocusedStyle

	return &AppModel{
		ag:        ag,
		modelName: modelName,
		agentName: fallback(func() string {
			if ag == nil {
				return ""
			}
			return ag.Name()
		}(), "Agent"),
		sessionID:        sessionID,
		promptDir:        promptDir,
		taskList:         taskList,
		skillMgr:         skillMgr,
		ctxManager:       ctxManager,
		messageCtx:       messageCtx,
		runTasks:         runTasks,
		switchModel:      switchModel,
		ctx:              ctx,
		viewport:         vp,
		input:            input,
		currentAssistant: -1,
		currentStatus:    "idle",
		autoScroll:       true,
		entries:          loadHistoryEntries(ctxManager, messageCtx),
	}
}

// loadHistoryEntries 从 ctxManager 加载历史消息到 conversationEntry
func loadHistoryEntries(ctxManager *agentctx.Manager, messageCtx *agentctx.Context) []conversationEntry {
	if ctxManager == nil || messageCtx == nil {
		return nil
	}

	messages, err := ctxManager.GetMessages(messageCtx)
	if err != nil || len(messages) == 0 {
		return nil
	}

	entries := make([]conversationEntry, 0, len(messages))
	for _, msg := range messages {
		var r string
		switch msg.Role {
		case schema.User:
			r = roleUser
		case schema.Assistant:
			if msg.ReasoningContent != "" {
				entries = append(entries, conversationEntry{Role: roleThinking, Content: msg.ReasoningContent})
			}
			r = roleAssistant
		case schema.System:
			r = roleSystem
		case schema.Tool:
			r = roleTool
		default:
			continue
		}
		entries = append(entries, conversationEntry{Role: r, Content: msg.Content})
	}
	return entries
}

func (m *AppModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.refreshView()
		return m, nil
	case assistantTokenMsg:
		if m.currentAssistant == -1 || m.currentAssistant >= len(m.entries) || m.entries[m.currentAssistant].Role != roleAssistant {
			m.entries = append(m.entries, conversationEntry{Role: roleAssistant, Content: msg.token})
			m.currentAssistant = len(m.entries) - 1
		} else if m.currentAssistant < len(m.entries) {
			m.entries[m.currentAssistant].Content += msg.token
		}
		m.currentStatus = "streaming"
		m.refreshView()
		return m, tickSpinner()
	case assistantThinkingMsg:
		if len(m.entries) > 0 && m.entries[len(m.entries)-1].Role == roleThinking {
			m.entries[len(m.entries)-1].Content += msg.token
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleThinking, Content: msg.token})
		}
		m.currentAssistant = -1
		m.currentStatus = "thinking"
		m.refreshView()
		return m, tickSpinner()
	case assistantDoneMsg:
		m.busy = false
		m.currentAssistant = -1
		m.currentStatus = "idle"
		m.refreshView()
		return m, nil
	case assistantErrorMsg:
		m.busy = false
		m.currentAssistant = -1
		m.currentStatus = "error"
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: "agent error: " + msg.err.Error()})
		m.refreshView()
		return m, nil
	case spinnerTickMsg:
		if m.busy {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			m.refreshView()
			return m, tickSpinner()
		}
		return m, nil
	case debugToolResultMsg:
		m.applyToolEvent(msg.event)
		m.currentStatus = "idle"
		m.refreshView()
		return m, nil
	case runTasksDoneMsg:
		m.busy = false
		m.currentAssistant = -1
		if msg.err != nil {
			m.currentStatus = "error"
			content := msg.err.Error()
			if strings.TrimSpace(msg.summary) != "" {
				content = strings.TrimSpace(msg.summary) + "\n\n" + content
			}
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: "/run", Content: content})
		} else {
			m.currentStatus = "idle"
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: "/run", Content: msg.summary})
		}
		m.refreshView()
		return m, nil
	case toolEventMsg:
		if msg.event.Kind == "call" {
			m.toolCalls++
			m.lastTool = fallback(tools.DisplayName(msg.event.Name), msg.event.Name)
		}
		m.applyToolEvent(msg.event)
		m.currentAssistant = -1
		m.refreshView()
		if msg.event.Kind == "call" {
			return m, tickSpinner()
		}
		return m, nil
	case tea.MouseMsg:
		// 鼠标滚轮只驱动历史 viewport，不抢输入框焦点。
		// tea.WithMouseCellMotion 负责把终端滚轮事件送到这里。
		before := m.viewport.YOffset
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if m.viewport.YOffset != before {
			m.autoScroll = m.viewport.AtBottom()
			return m, cmd
		}
		return m, cmd
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.escPending && time.Since(m.lastEscAt) <= 700*time.Millisecond {
				return m, tea.Quit
			}
			m.escPending = true
			m.lastEscAt = time.Now()
			m.currentStatus = "esc again to quit"
			m.refreshView()
			return m, nil
		case "enter":
			return m, m.submit()
		case "pgdown", "ctrl+f":
			m.autoScroll = m.viewport.AtBottom()
			m.viewport.ViewDown()
			m.autoScroll = m.viewport.AtBottom()
			m.refreshView()
			return m, nil
		case "pgup", "ctrl+b":
			m.autoScroll = false
			m.viewport.ViewUp()
			m.refreshView()
			return m, nil
		case "down", "ctrl+n":
			m.viewport.LineDown(1)
			m.autoScroll = m.viewport.AtBottom()
			m.refreshView()
			return m, nil
		case "up", "ctrl+p":
			m.autoScroll = false
			m.viewport.LineUp(1)
			m.refreshView()
			return m, nil
		case "end":
			m.viewport.GotoBottom()
			m.autoScroll = true
			m.refreshView()
			return m, nil
		case "home":
			m.viewport.GotoTop()
			m.autoScroll = false
			m.refreshView()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.escPending = false
	m.input, cmd = m.input.Update(msg)
	m.viewport, _ = m.viewport.Update(msg)
	m.autoScroll = m.viewport.AtBottom()
	return m, cmd
}

func (m *AppModel) View() string {
	m.resize()
	// 当前布局保持单列：历史记录在上，输入框附近承载运行状态。
	mainView := renderMainPane(m)
	statusBar := renderBottomStatusBar(m.width, m.currentStatus, m.busy)
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Height(max(1, m.height-1)).Render(mainView),
		statusBar,
	)
}

func (m *AppModel) submit() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	if m.busy {
		m.currentStatus = "busy"
		return nil
	}

	m.input.Reset()
	m.entries = append(m.entries, conversationEntry{Role: roleUser, Content: text})
	m.refreshView()

	if strings.HasPrefix(text, "/debug") && os.Getenv("5HAGENT_TUI_DEBUG") == "1" {
		return m.handleDebugCommand(text)
	}

	if strings.HasPrefix(text, "/skill") {
		result, err := commands.HandleSkill(text, m.skillMgr)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/task") {
		result, err := commands.HandleTask(text, m.taskList)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/compress") {
		result, err := commands.HandleCompress(m.ctx, text, m.ctxManager, m.messageCtx, m.ag.GetModel(), m.promptDir, "compact")
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/mcp") {
		result, err := commands.HandleMCP(text)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/session") {
		result, err := m.handleSessionCommand(text)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, SystemTitle: text, Content: result})
		}
		m.refreshView()
		return nil
	}

	if fields := strings.Fields(text); len(fields) > 0 && fields[0] == "/model" {
		return m.handleModelCommand(text)
	}

	if fields := strings.Fields(text); len(fields) > 0 && fields[0] == "/run" {
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
	m.refreshView()

	go m.runAgent(text)
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

func (m *AppModel) runAgent(input string) {
	if m.program == nil {
		return
	}
	_, err := m.ag.RunStream(m.ctx, m.messageCtx, input, func(token string) {
		m.program.Send(assistantTokenMsg{token: token})
	}, func(token string) {
		m.program.Send(assistantThinkingMsg{token: token})
	})
	if err != nil {
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

func (m *AppModel) resize() {
	if m.width <= 0 {
		m.width = defaultTUIWidth
	}
	if m.height <= 0 {
		m.height = defaultTUIHeight
	}

	mainWidth := max(20, m.width)
	headerHeight := 0
	footerHeight := 1
	inputAreaHeight := 5
	if strings.HasPrefix(strings.TrimSpace(m.input.Value()), "/") {
		inputAreaHeight += len(slashHintMatches(strings.TrimSpace(m.input.Value())))
	}
	m.viewport.Width = max(8, mainWidth)
	m.viewport.Height = max(1, m.height-headerHeight-footerHeight-inputAreaHeight)
	m.input.SetWidth(max(8, mainWidth-2))
	m.input.SetHeight(2)
}

func (m *AppModel) refreshView() {
	m.resize()
	contentWidth := max(8, m.viewport.Width)
	stickToBottom := m.autoScroll || m.viewport.AtBottom() || m.viewport.TotalLineCount() <= m.viewport.Height
	parts := make([]string, 0, len(m.entries))
	for _, entry := range m.entries {
		parts = append(parts, m.renderConversationEntry(entry, contentWidth))
	}
	if len(parts) == 0 {
		parts = append(parts, renderEmptyState(contentWidth))
	}
	m.viewText = strings.Join(parts, "\n")
	m.viewport.SetContent(m.viewText)
	if stickToBottom {
		m.viewport.GotoBottom()
		m.autoScroll = true
	}
}

func renderEmptyState(width int) string {
	lines := []string{
		lipgloss.NewStyle().Foreground(colorYellow).Render("5hAgent ready"),
		lipgloss.NewStyle().Foreground(colorMuted).Render("type a prompt to start"),
		lipgloss.NewStyle().Foreground(colorMuted).Render("type / for commands"),
	}
	for i, line := range lines {
		lines[i] = truncateMiddle(line, max(20, width-2))
	}
	return strings.Join(lines, "\n")
}

// snapshot 汇总输入框附近状态区需要的数据。
// 调用层级：View -> renderMainPane -> snapshot。
// 主要步骤：读取 token、context、skill、task 的只读摘要；不在渲染函数里直接散落业务查询。
func (m *AppModel) snapshot() statusSnapshot {
	used, limit := 0, 0
	turn := 0
	if m.ag != nil {
		used, limit = m.ag.TokenUsage()
		turn = m.ag.CurrentTurn()
	}
	snapshot := statusSnapshot{Runtime: runtimeMeta{
		Busy:           m.busy,
		State:          animatedStateLabel(m.busy, m.currentStatus, m.spinnerFrame),
		Turn:           turn,
		TokenUsed:      used,
		TokenLimit:     limit,
		ScrollPercent:  int(m.viewport.ScrollPercent() * 100),
		ToolCallsTotal: m.toolCalls,
		LastToolName:   m.lastTool,
		SessionID:      m.sessionID,
	}}
	snapshot.Runtime.Workdir, snapshot.Runtime.Git = m.loadRuntimeLocation()
	if m.ctxManager != nil && m.messageCtx != nil {
		if messages, err := m.ctxManager.GetMessages(m.messageCtx); err == nil {
			snapshot.Runtime.ContextMessages = len(messages)
			for _, msg := range messages {
				if strings.HasPrefix(msg.Content, "[对话历史摘要]") {
					snapshot.Runtime.ContextSummaries++
				}
			}
		}
	}
	if m.skillMgr != nil {
		for _, s := range m.skillMgr.ListSkills() {
			if s.Enabled {
				snapshot.EnabledSkills = append(snapshot.EnabledSkills, s.Name)
			}
		}
		sort.Strings(snapshot.EnabledSkills)
	}
	if m.taskList != nil {
		total, _, inProgress, _, _, _, _ := m.taskList.GetProgress()
		snapshot.Runtime.TotalTasks = total
		snapshot.Runtime.ActiveTasks = inProgress
		tasks := m.taskList.ListTasksByStatus(task.StatusInProgress)
		for i, task := range tasks {
			if i >= 3 {
				break
			}
			snapshot.HighlightedTaskLine = append(snapshot.HighlightedTaskLine, fmt.Sprintf("%s %s", task.ID, task.Status))
		}
	}
	return snapshot
}

func (m *AppModel) loadRuntimeLocation() (string, gitMeta) {
	if time.Since(m.metaCache.LoadedAt) < 2*time.Second && m.metaCache.Workdir != "" {
		return m.metaCache.Workdir, m.metaCache.Git
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "-"
	}
	meta := readGitMeta(cwd)
	m.metaCache = cachedMeta{Workdir: cwd, Git: meta, LoadedAt: time.Now()}
	return cwd, meta
}

func readGitMeta(dir string) gitMeta {
	root := gitOutput(dir, "rev-parse", "--show-toplevel")
	if root == "" {
		return gitMeta{}
	}
	gitDir := gitOutput(dir, "rev-parse", "--git-dir")
	branch := gitOutput(dir, "branch", "--show-current")
	if branch == "" {
		branch = gitOutput(dir, "rev-parse", "--short", "HEAD")
	}
	status := gitOutput(dir, "status", "--porcelain")
	shortstat := gitOutput(dir, "diff", "--shortstat")
	return gitMeta{
		Repo:      true,
		Worktree:  gitDir != "" && !filepath.IsAbs(gitDir),
		Branch:    branch,
		Dirty:     strings.TrimSpace(status) != "",
		Shortstat: compactShortstat(shortstat),
	}
}

func gitOutput(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func compactShortstat(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, " files changed", " files")
	text = strings.ReplaceAll(text, " file changed", " file")
	text = strings.ReplaceAll(text, " insertions(+)", "+")
	text = strings.ReplaceAll(text, " insertion(+)", "+")
	text = strings.ReplaceAll(text, " deletions(-)", "-")
	text = strings.ReplaceAll(text, " deletion(-)", "-")
	text = strings.ReplaceAll(text, ",", "")
	return strings.Join(strings.Fields(text), " ")
}

// renderMainPane 渲染单列 TUI 主体。
// 调用层级：View -> renderMainPane -> renderTopStatus/renderViewportPane/renderInputFooter。
// 布局：顶部运行状态、历史 viewport、slash hint、亮灰输入框、底部补充状态。
func renderMainPane(m *AppModel) string {
	width := max(20, m.width)
	snapshot := m.snapshot()
	header := renderTopStatus(snapshot, m.modelName, m.agentName, width)
	conversationHeight := max(1, m.viewport.Height)
	conversation := lipgloss.NewStyle().Height(conversationHeight).Render(renderViewportPane(m.viewport, m.viewText))
	inputBlock := renderInputBar(m.input.View(), width)
	slashHint := m.renderSlashHint(max(12, width-4))
	footer := renderInputFooter(snapshot, m.sessionID, width)
	content := lipgloss.JoinVertical(lipgloss.Left,
		conversation,
		header,
		slashHint,
		inputBlock,
		footer,
	)
	return mainViewStyle.Width(width).Render(content)
}

// renderInputBar 使用参考 tmux 对话窗口的上下深灰线样式包住输入行。
// 参数：inputView 是 textarea 当前输出；width 是终端主列宽度。
func renderInputBar(inputView string, width int) string {
	width = max(12, width)
	line := lipgloss.NewStyle().Foreground(colorInputBg).Render(strings.Repeat("▄", width))
	bottom := lipgloss.NewStyle().Foreground(colorInputBg).Render(strings.Repeat("▀", width))
	body := inputShellStyle.Width(width).Render(compactInputView(inputView))
	return strings.Join([]string{line, body, bottom}, "\n")
}

func compactInputView(inputView string) string {
	lines := strings.Split(inputView, "\n")
	for len(lines) > 1 && isEmptyInputPromptLine(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func isEmptyInputPromptLine(line string) bool {
	plain := strings.TrimSpace(stripANSI(line))
	return plain == "" || plain == ">"
}

// renderTopStatus 渲染输入框上方的高频运行状态。
// 参数：snapshot 为运行快照；modelName/agentName 来自 AppModel；width 为当前主列宽度。
func renderTopStatus(snapshot statusSnapshot, modelName string, _ string, width int) string {
	meta := snapshot.Runtime
	if width < 72 {
		parts := []string{
			lipgloss.NewStyle().Foreground(colorCommand).Render(truncateMiddle(fallback(modelName, "-"), 22)),
			renderState(meta.State, meta.Busy),
		}
		if meta.Turn > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("turn %d", meta.Turn)))
		}
		if meta.ToolCallsTotal > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render(fmt.Sprintf("tools %d", meta.ToolCallsTotal)))
		}
		return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
	}
	parts := []string{
		lipgloss.NewStyle().Foreground(colorCommand).Render(truncateMiddle(fallback(modelName, "-"), max(16, width/3))),
		renderState(meta.State, meta.Busy),
	}
	if meta.Turn > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("turn %d", meta.Turn)))
	}
	if meta.ToolCallsTotal > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render(fmt.Sprintf("tools %d", meta.ToolCallsTotal)))
	}
	if meta.LastToolName != "" {
		last := truncateMiddle(fallback(tools.DisplayName(meta.LastToolName), meta.LastToolName), 24)
		parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render("last "+last))
	}
	if meta.TokenUsed > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("tok %d/%d", meta.TokenUsed, meta.TokenLimit)))
	}
	return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
}

// renderInputFooter 渲染输入框下方的低频上下文状态。
// 参数：snapshot 为运行快照；sessionID 用于显示当前会话；width 为当前主列宽度。
func renderInputFooter(snapshot statusSnapshot, sessionID string, width int) string {
	meta := snapshot.Runtime
	shortSession := truncateMiddle(fallback(meta.SessionID, sessionID), 12)
	if width < 72 {
		parts := []string{
			lipgloss.NewStyle().Foreground(colorMuted).Render("session " + shortSession),
		}
		if meta.Git.Repo {
			branch := truncateMiddle(fallback(meta.Git.Branch, "detached"), 12)
			if meta.Git.Dirty {
				branch += "*"
			}
			if meta.Git.Worktree {
				parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render("worktree "+branch))
			} else {
				parts = append(parts, lipgloss.NewStyle().Foreground(colorBlue).Render("git "+branch))
			}
		}
		if meta.TotalTasks > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorCommand).Render(fmt.Sprintf("tasks %d active / %d total", meta.ActiveTasks, meta.TotalTasks)))
		}
		if meta.ScrollPercent < 100 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("scroll %d%%", meta.ScrollPercent)))
		}
		return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
	}
	parts := []string{
		lipgloss.NewStyle().Foreground(colorMuted).Render("session " + shortSession),
	}
	if meta.Workdir != "" && meta.Workdir != "-" {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorWhite).Render("dir "+truncateMiddle(filepath.Base(meta.Workdir), 18)))
	}
	if meta.Git.Repo {
		branch := truncateMiddle(fallback(meta.Git.Branch, "detached"), 18)
		prefix := "git "
		color := colorBlue
		if meta.Git.Worktree {
			prefix = "worktree "
			color = colorGreen
		}
		if meta.Git.Dirty {
			branch += "*"
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(color).Render(prefix+branch))
		if meta.Git.Shortstat != "" {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorError).Render("diff "+truncateMiddle(meta.Git.Shortstat, 20)))
		}
	}
	if meta.ContextMessages > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("msgs %d", meta.ContextMessages)))
	}
	if meta.ContextSummaries > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("sum %d", meta.ContextSummaries)))
	}
	if meta.TotalTasks > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorCommand).Render(fmt.Sprintf("tasks %d active / %d total", meta.ActiveTasks, meta.TotalTasks)))
	}
	if meta.ScrollPercent < 100 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorPurple).Render(fmt.Sprintf("scroll %d%%", meta.ScrollPercent)))
	}
	if len(snapshot.EnabledSkills) > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render(skillSummary(snapshot.EnabledSkills)))
	}
	if len(snapshot.HighlightedTaskLine) > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorCommand).Render("focus "+truncateMiddle(strings.Join(snapshot.HighlightedTaskLine, ","), 48)))
	}
	return strings.Join(parts, lipgloss.NewStyle().Faint(true).Render(" · "))
}

func renderState(state string, busy bool) string {
	color := colorGreen
	if busy {
		color = colorYellow
	}
	if strings.Contains(state, "error") {
		color = colorError
	}
	return lipgloss.NewStyle().Foreground(color).Render(state)
}

func skillSummary(skills []string) string {
	if len(skills) == 0 {
		return ""
	}
	if len(skills) == 1 {
		return "skill " + truncateMiddle(skills[0], 24)
	}
	return fmt.Sprintf("skills %d (%s)", len(skills), truncateMiddle(skills[0], 18))
}

func (m *AppModel) renderSlashHint(width int) string {
	text := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	matches := slashHintMatches(text)
	if len(matches) == 0 {
		return slashHintStyle.Width(width).Render("unknown slash command")
	}
	parts := make([]string, 0, len(matches))
	for _, hint := range matches {
		text := hint.Usage + "  " + hint.Desc
		if width < 72 {
			text = hint.Name + "  " + hint.Desc
		}
		parts = append(parts, wrapVisibleText(text, max(8, width-2)))
	}
	return slashHintStyle.Width(width).Render(strings.Join(parts, "\n"))
}

func slashHintMatches(input string) []slashCommandHint {
	fields := strings.Fields(input)
	prefix := input
	if len(fields) > 0 {
		prefix = fields[0]
	}
	if prefix == "/" {
		return slashCommandHints
	}
	matches := make([]slashCommandHint, 0, len(slashCommandHints))
	for _, hint := range slashCommandHints {
		if strings.HasPrefix(hint.Name, prefix) {
			matches = append(matches, hint)
		}
	}
	return matches
}

func renderBottomStatusBar(width int, status string, busy bool) string {
	state := fallback(status, "idle")
	if busy {
		state = animatedStateLabel(busy, status, 0)
	}
	if state == "idle" {
		return statusBarStyle.Width(max(1, width)).Render(strings.Repeat(" ", max(1, width)))
	}
	text := " " + lipgloss.NewStyle().Foreground(colorMuted).Render(state)
	padding := width - lipgloss.Width(text)
	if padding < 0 {
		padding = 0
	}
	return statusBarStyle.Width(max(1, width)).Render(text + strings.Repeat(" ", padding))
}

func (m *AppModel) findToolEntry(name string) int {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Role != roleTool || !m.entries[i].ToolOpen {
			continue
		}
		if name == "" || m.entries[i].ToolName == name {
			return i
		}
	}
	return -1
}

func (m *AppModel) applyToolEvent(event logger.ToolEvent) {
	displayName := fallback(tools.DisplayName(event.Name), event.Name)
	key := toolEventKey(event.Name, event.Args)
	summary := formatToolArgsSummary(event.Args)

	switch event.Kind {
	case "call":
		m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "running", ToolOutput: "running..."})
	case "result":
		idx := m.findRunningToolEntry(key, event.Name)
		output := summarizeToolEventOutput(event)
		if idx < 0 {
			m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "done", ToolOutput: output})
			return
		}
		m.entries[idx].ToolState = "done"
		m.entries[idx].ToolOutput = output
	case "error":
		idx := m.findRunningToolEntry(key, event.Name)
		output := summarizeToolEventOutput(event)
		if idx < 0 {
			m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "error", ToolOutput: output})
			return
		}
		m.entries[idx].ToolState = "error"
		m.entries[idx].ToolOutput = output
	default:
		m.entries = append(m.entries, conversationEntry{Role: roleHint, ToolName: displayName, ToolArgs: summary, ToolKey: key, ToolState: "done", ToolOutput: strings.TrimSpace(stripANSI(event.Text))})
	}
}

func (m *AppModel) findRunningToolEntry(key string, name string) int {
	displayName := tools.DisplayName(name)
	for i := len(m.entries) - 1; i >= 0; i-- {
		entry := m.entries[i]
		if entry.Role != roleHint || entry.ToolState != "running" {
			continue
		}
		if key != "" && entry.ToolKey == key {
			return i
		}
		if entry.ToolName == name || entry.ToolName == displayName {
			return i
		}
	}
	return -1
}

func toolEventKey(name string, args string) string {
	return name + "\x00" + strings.TrimSpace(args)
}

func formatToolArgsSummary(args string) string {
	args = strings.TrimSpace(args)
	if args == "" || args == "{}" {
		return ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return "[args=" + truncateMiddle(args, 120) + "]"
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+truncateMiddle(toolArgValue(raw[key]), 80))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func toolArgValue(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		return fmt.Sprintf("%g", vv)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func summarizeToolEventOutput(event logger.ToolEvent) string {
	clean := strings.TrimSpace(stripANSI(event.Text))
	entry := parseToolBlock(clean)
	switch event.Kind {
	case "result":
		fields := append([]string{}, entry.Result...)
		if len(fields) == 0 {
			fields = strings.Split(strings.TrimSpace(strings.TrimPrefix(clean, "⎿ ")), "\n")
		}
		return compactOutputLines(fields, 4)
	case "error":
		if entry.Error != "" {
			return compactOutputLines([]string{entry.Error}, 2)
		}
		if event.Error != "" {
			return "error: " + truncateMiddle(event.Error, 180)
		}
		return compactOutputLines(strings.Split(clean, "\n"), 2)
	default:
		return compactOutputLines(strings.Split(clean, "\n"), 3)
	}
}

func compactOutputLines(lines []string, limit int) string {
	result := make([]string, 0, limit)
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "⎿ "))
		if line == "" {
			continue
		}
		result = append(result, truncateMiddle(line, 160))
		if len(result) >= limit {
			break
		}
	}
	if len(result) == 0 {
		return "(no output)"
	}
	if len(lines) > len(result) {
		result = append(result, "...")
	}
	return strings.Join(result, "\n")
}

func truncateInline(text string, maxLen int) string {
	text = strings.Join(strings.Fields(text), " ")
	return truncateMiddle(text, maxLen)
}

func cleanDisplayText(text string) string {
	return strings.ReplaceAll(text, "\uFFFD", "")
}

func (m *AppModel) renderConversationEntry(entry conversationEntry, width int) string {
	switch entry.Role {
	case roleUser:
		body := compactParagraph(strings.TrimSpace(entry.Content))
		return renderUserEntry(body, width)
	case roleAssistant:
		content := strings.TrimRight(renderMarkdownForTerminal(normalizeAssistantContent(entry.Content), true), "\n")
		return wrapVisibleText(content, max(8, width))
	case roleHint:
		return m.renderToolHintEntry(entry, width)
	case roleThinking:
		return renderThinkingEntry(entry.Content, width)
	case roleTool:
		return renderToolEntry(entry.Content, width)
	case roleSystem:
		return renderSystemEntry(entry.SystemTitle, entry.Content, width)
	default:
		return wrapVisibleText(strings.TrimSpace(entry.Content), width)
	}
}

func renderSystemEntry(title string, content string, width int) string {
	content = strings.TrimSpace(content)
	label := "System"
	if strings.TrimSpace(title) != "" {
		label = "Command " + truncateMiddle(strings.TrimSpace(title), 40)
		content = compactCommandOutput(content, 12)
	}
	if content == "" {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("◆ " + label)
	}
	prefix := lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("◆") + " " + lipgloss.NewStyle().Bold(true).Render(label)
	body := loggerColorLines(wrapVisibleText(content, max(8, width-4)), colorMuted)
	return prefix + "\n" + indentLines(body, "  └ ", "    ")
}

func compactCommandOutput(content string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) <= maxLines || maxLines < 4 {
		return content
	}
	tail := strings.TrimSpace(lines[len(lines)-1])
	headCount := maxLines - 2
	result := append([]string{}, lines[:headCount]...)
	result = append(result, fmt.Sprintf("... %d lines omitted ...", len(lines)-maxLines+1))
	if tail != "" {
		result = append(result, tail)
	}
	return strings.Join(result, "\n")
}

func renderUserEntry(content string, width int) string {
	content = strings.TrimSpace(content)
	prefix := "▍ "
	style := lipgloss.NewStyle().Foreground(colorWhite).Background(colorInputBg)
	if content == "" {
		return style.Render(prefix)
	}
	lineWidth := max(8, width-lipgloss.Width(prefix))
	lines := wrapVisibleLines(content, lineWidth)
	for i, line := range lines {
		if i == 0 {
			lines[i] = style.Render(prefix + line)
			continue
		}
		lines[i] = style.Render(strings.Repeat(" ", lipgloss.Width(prefix)) + line)
	}
	return strings.Join(lines, "\n")
}

func renderThinkingEntry(content string, width int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	label := lipgloss.NewStyle().Foreground(colorMuted).Faint(true).Render("◆ thinking")
	body := loggerColorLines(wrapVisibleText(content, max(8, width-2)), colorGray)
	return label + "\n" + indentLines(body, "  └ ", "    ")
}

func colorLinesANSI(text string, color func(string) string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = color(line)
	}
	return strings.Join(lines, "\n")
}

func (m *AppModel) renderToolHintEntry(entry conversationEntry, width int) string {
	stateIcon := "●"
	stateColor := colorGreen
	switch entry.ToolState {
	case "running":
		stateIcon = spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		stateColor = colorBlue
	case "error":
		stateIcon = "●"
		stateColor = colorError
	}

	name := fallback(entry.ToolName, "tool")
	args := strings.TrimSpace(entry.ToolArgs)
	if entry.ToolState != "running" {
		stateIcon = "◆"
	}
	header := lipgloss.NewStyle().Foreground(stateColor).Bold(true).Render(stateIcon) + " " + lipgloss.NewStyle().Bold(true).Render(toolDisplayVerb(entry.ToolState)) + " " + lipgloss.NewStyle().Foreground(colorCommand).Render(name)
	if args != "" {
		header += " " + logger.Gray(args)
	}

	output := strings.TrimSpace(entry.ToolOutput)
	if output == "" {
		return header
	}
	lineWidth := max(8, width-4)
	lines := wrapVisibleText(output, lineWidth)
	if entry.ToolState == "error" {
		lines = loggerColorLines(lines, colorError)
	} else {
		lines = loggerColorLines(lines, colorResult)
	}
	return header + "\n" + indentLines(lines, "  └ ", "  └ ")
}

func toolDisplayVerb(state string) string {
	switch state {
	case "running":
		return "Running"
	case "error":
		return "Failed"
	default:
		return "Ran"
	}
}

func loggerColorLines(text string, color lipgloss.Color) string {
	style := lipgloss.NewStyle().Foreground(color)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = style.Render(line)
	}
	return strings.Join(lines, "\n")
}

func indentLines(text string, firstPrefix string, nextPrefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = firstPrefix + line
		} else {
			lines[i] = nextPrefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func renderPrefixedPlainText(prefix string, content string, color lipgloss.Color, width int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return lipgloss.NewStyle().Foreground(color).Render(prefix)
	}
	lineWidth := max(8, width-lipgloss.Width(prefix))
	lines := wrapVisibleLines(content, lineWidth)
	for i, line := range lines {
		if i == 0 {
			lines[i] = lipgloss.NewStyle().Foreground(color).Render(prefix) + line
			continue
		}
		lines[i] = strings.Repeat(" ", lipgloss.Width(prefix)) + line
	}
	return strings.Join(lines, "\n")
}

func renderMessageBlock(label string, body string, accent lipgloss.Color, width int) string {
	blockWidth := max(8, width-1)
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(label)
	content := lipgloss.NewStyle().Foreground(colorText).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		messageBoxStyle.Copy().Width(blockWidth).Render(content),
	)
}

func wrapVisibleLines(line string, width int) []string {
	if width <= 0 || lipgloss.Width(line) <= width || strings.TrimSpace(stripANSI(line)) == "" {
		return []string{line}
	}
	if line != stripANSI(line) {
		line = stripANSI(line)
	}
	lines := strings.Split(ansi.Wrap(line, width, " \t"), "\n")
	if len(lines) == 0 {
		return []string{line}
	}
	return lines
}

func wrapVisibleText(text string, width int) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines)*2)
	for _, line := range lines {
		if line == "" {
			result = append(result, "")
			continue
		}
		wrapped := wrapVisibleLines(line, width)
		result = append(result, wrapped...)
	}
	return strings.Join(result, "\n")
}

func truncateVisibleLine(text string, width int) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}
	return truncateMiddle(lines[0], max(1, width))
}

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

func renderViewportPane(vp viewport.Model, content string) string {
	width := max(1, vp.Width)
	height := max(1, vp.Height)
	var lines []string
	if vp.TotalLineCount() <= vp.Height {
		lines = strings.Split(content, "\n")
		for len(lines) < height {
			lines = append([]string{""}, lines...)
		}
	} else {
		lines = strings.Split(vp.View(), "\n")
	}
	return renderFixedLines(lines, width, height)
}

func renderFixedLines(lines []string, width int, height int) string {
	fixed := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		line = truncateMiddle(line, width)
		padding := width - lipgloss.Width(line)
		if padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		fixed = append(fixed, line)
	}
	return strings.Join(fixed, "\n")
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// toolEntry 表示解析后的工具条目
type toolEntry struct {
	Name   string
	Args   []string
	Result []string
	Error  string
}

func parseToolBlock(text string) toolEntry {
	clean := strings.TrimSpace(ansiPattern.ReplaceAllString(text, ""))
	if clean == "" {
		return toolEntry{}
	}

	lines := strings.Split(clean, "\n")
	entry := toolEntry{}
	seenResult := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "● ") {
			rest := strings.TrimPrefix(line, "● ")
			rest = strings.TrimSuffix(rest, " [并发]")
			entry.Name = extractToolNameFromCall(rest)
			continue
		}
		if strings.HasPrefix(line, "⎿ ") {
			seenResult = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "⎿ "))
		}
		if strings.HasPrefix(line, "✗") {
			entry.Error = strings.TrimSpace(line)
			seenResult = true
			continue
		}
		if !seenResult {
			entry.Args = append(entry.Args, line)
			continue
		}
		entry.Result = append(entry.Result, line)
	}
	return entry
}

// extractToolName 从工具调用行提取工具名称
func extractToolNameFromCall(text string) string {
	// 格式: ToolName(args...) 或 ToolName
	text = strings.TrimSpace(text)
	// 找到第一个括号的位置
	if idx := strings.Index(text, "("); idx > 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
}

func renderToolEntry(content string, width int) string {
	entry := parseToolBlock(content)
	if entry.Name == "" && len(entry.Args) == 0 && len(entry.Result) == 0 && entry.Error == "" {
		return renderToolUnknownEntry(content, width)
	}
	return renderToolCompactEntry(entry, width)
}

func renderToolCompactEntry(entry toolEntry, width int) string {
	var b strings.Builder
	icon := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("◆")
	name := lipgloss.NewStyle().Foreground(colorCommand).Render(fallback(entry.Name, "tool"))
	b.WriteString(icon + " " + lipgloss.NewStyle().Bold(true).Render("Ran") + " " + name)

	if len(entry.Args) > 0 {
		joined := strings.Join(entry.Args, "   ·   ")
		wrapped := wrapVisibleLines(joined, max(8, width-2))
		b.WriteString("\n")
		for _, wl := range wrapped {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Faint(true).Render("  │ " + wl))
			b.WriteString("\n")
		}
	}

	if entry.Error != "" {
		b.WriteString("\n")
		for _, wl := range wrapVisibleLines(entry.Error, max(8, width-2)) {
			b.WriteString(lipgloss.NewStyle().Foreground(colorError).Render("  └ " + wl))
			b.WriteString("\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}

	if len(entry.Result) > 0 {
		showLines := entry.Result
		truncated := false
		if len(showLines) > 2 {
			showLines = showLines[:2]
			truncated = true
		}
		if len(entry.Args) == 0 {
			b.WriteString("\n")
		}
		for _, line := range showLines {
			wrapped := wrapVisibleLines(truncateMiddle(line, max(24, width+12)), max(8, width-2))
			for _, wl := range wrapped {
				b.WriteString(lipgloss.NewStyle().Foreground(colorResult).Render("  └ " + wl))
				b.WriteString("\n")
			}
		}
		if truncated {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  └ ..."))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderToolUnknownEntry 渲染未知工具条目
func renderToolUnknownEntry(content string, width int) string {
	clean := strings.TrimSpace(ansiPattern.ReplaceAllString(content, ""))
	if clean == "" {
		return lipgloss.NewStyle().Foreground(colorGray).Render("◆ Ran tool\n  └ (empty)")
	}
	maxLen := width - 10
	if maxLen < 20 {
		maxLen = 20
	}
	return lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("◆") + " " + lipgloss.NewStyle().Bold(true).Render("Ran") + " " + lipgloss.NewStyle().Foreground(colorGray).Render(truncateMiddle(clean, maxLen))
}

func renderMessageBoxWithHeader(text string, width int) string {
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) == 1 {
		return renderMessageBlock(lines[0], "", colorYellow, width)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		lines[0],
		messageBoxStyle.Copy().Width(max(8, width-1)).Render(lipgloss.NewStyle().Foreground(colorText).Render(lines[1])),
	)
}

// truncateMiddle 截断中间部分
func truncateMiddle(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	half := (maxLen - 3) / 2
	return string(runes[:half]) + "..." + string(runes[len(runes)-half:])
}

func normalizeAssistantContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	for strings.Contains(content, "\n\n") {
		content = strings.ReplaceAll(content, "\n\n", "\n")
	}
	return content
}

func animatedStateLabel(busy bool, state string, frame int) string {
	state = fallback(state, "idle")
	if !busy || state == "idle" || len(spinnerFrames) == 0 {
		return state
	}
	return spinnerFrames[frame%len(spinnerFrames)] + " " + state
}

func tickSpinner() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func LaunchTUI(ctx context.Context, ag *agent.Agent, modelName string, promptDir string, taskList *task.TaskList, skillMgr *skill.Manager, ctxManager *agentctx.Manager, messageCtx *agentctx.Context, sessionID string, runTasks RunTasksFunc, switchModel SwitchModelFunc) error {
	launchMu.Lock()
	defer launchMu.Unlock()
	model := NewAppModel(ctx, ag, modelName, promptDir, taskList, skillMgr, ctxManager, messageCtx, sessionID, runTasks, switchModel)
	// WithMouseCellMotion 开启点击、释放和滚轮事件；viewport.Update 负责具体滚动。
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	model.program = p
	prevSink := ag.SetToolEventSink(func(event logger.ToolEvent) {
		p.Send(toolEventMsg{event: event})
	})
	defer ag.SetToolEventSink(prevSink)
	_, err := p.Run()
	return err
}
