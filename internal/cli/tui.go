// tui.go - 终端 UI 主界面
// 功能：Bubble Tea 构建的交互式对话界面，显示对话/状态面板，支持 /task /skill /compress 命令
// 主要类型：AppModel, conversationEntry, statusSnapshot
// 导出函数：NewAppModel, LaunchTUI
package cli

import (
	"context"
	"fmt"
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
	defaultTUIWidth  = 100
	defaultTUIHeight = 30
)

type conversationEntry struct {
	Role     string
	Content  string
	ToolName string
	ToolOpen bool
}

type statusSnapshot struct {
	Busy                bool
	CurrentState        string
	TokenUsed           int
	TokenLimit          int
	ContextMessages     int
	ContextSummaries    int
	ToolCallsTotal      int
	LastToolName        string
	EnabledSkills       []string
	TaskTotal           int
	TaskInProgress      int
	HighlightedTaskLine []string
}

type AppModel struct {
	program    *tea.Program
	ag         *agent.Agent
	modelName  string
	agentName  string
	sessionID  string
	promptDir  string
	taskList   *task.TaskList
	skillMgr   *skill.Manager
	ctxManager *agentctx.Manager
	messageCtx *agentctx.Context
	ctx        context.Context

	width       int
	height      int
	statusWidth int
	busy        bool

	viewport  viewport.Model
	input     textarea.Model
	entries   []conversationEntry
	toolCalls int
	lastTool  string

	currentAssistant int
	currentStatus    string
	spinnerFrame     int
	sidebarCursor    int
	escPending       bool
	lastEscAt        time.Time
	autoScroll       bool
}

type assistantTokenMsg struct {
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

	sidebarStyle = lipgloss.NewStyle().
			Width(24).
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(colorSurface).
			Padding(0, 1)

	infoPanelStyle = lipgloss.NewStyle().
			Width(30).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(colorSurface).
			Padding(0, 1)

	mainViewStyle = lipgloss.NewStyle().
			Padding(0, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	sectionTitleStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Bold(true).
				MarginTop(1).
				MarginBottom(1)

	navItemStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			PaddingLeft(2)

	activeNavItemStyle = navItemStyle.Copy().
				Foreground(colorBg).
				Background(colorGreen).
				Bold(true)

	statusBarStyle = lipgloss.NewStyle().
			Background(colorBlue).
			Foreground(colorBg).
			Bold(true)

	inputShellStyle = lipgloss.NewStyle().
			Background(colorBlack).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSurface).
			Padding(0, 1)

	messageBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSurface).
			Padding(0, 1)
)

var menuItems = []string{"CHATS", "HISTORY", "LOGS", "AGENTS"}

func NewAppModel(ctx context.Context, ag *agent.Agent, modelName string, promptDir string, taskList *task.TaskList, skillMgr *skill.Manager, ctxManager *agentctx.Manager, messageCtx *agentctx.Context, sessionID string) *AppModel {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 2

	input := textarea.New()
	input.Placeholder = ""
	input.Focus()
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.Prompt = "> "
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorBlue).Background(colorSurface).Bold(true)
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorText).Background(colorSurface)
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorGray).Background(colorSurface)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(colorText).Background(colorSurface)
	input.FocusedStyle.CursorLineNumber = lipgloss.NewStyle().Foreground(colorGray).Background(colorSurface)
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
		ctx:              ctx,
		viewport:         vp,
		input:            input,
		currentAssistant: -1,
		currentStatus:    "idle",
		sidebarCursor:    0,
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
	case toolEventMsg:
		text := compactToolEventText(msg.event)
		if msg.event.Kind == "call" {
			m.toolCalls++
			m.lastTool = fallback(tools.DisplayName(msg.event.Name), msg.event.Name)
		}
		m.entries = append(m.entries, conversationEntry{Role: roleHint, Content: text})
		m.currentAssistant = -1
		m.refreshView()
		return m, nil
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
			return m, nil
		case "pgup", "ctrl+b":
			m.autoScroll = false
			m.viewport.ViewUp()
			return m, nil
		case "down", "j", "ctrl+n":
			m.viewport.LineDown(1)
			m.autoScroll = m.viewport.AtBottom()
			return m, nil
		case "up", "k", "ctrl+p":
			m.autoScroll = false
			m.viewport.LineUp(1)
			return m, nil
		case "end":
			m.viewport.GotoBottom()
			m.autoScroll = true
			return m, nil
		case "home":
			m.viewport.GotoTop()
			m.autoScroll = false
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
	sidebar := renderSidebar(m.sidebarCursor, m.height)
	rightPanel := infoPanelStyle.Width(m.statusWidth).Height(max(1, m.height-1)).Render(renderStatusPanel(m.snapshot(), max(12, m.statusWidth-2)))
	mainView := renderMainPane(m)
	layout := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainView, rightPanel)
	statusBar := renderBottomStatusBar(m.width, m.currentStatus, m.busy)
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Height(max(1, m.height-1)).Render(layout),
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

	if strings.HasPrefix(text, "/skill") {
		result, err := commands.HandleSkill(text, m.skillMgr)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/task") {
		result, err := commands.HandleTask(text, m.taskList)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/compress") {
		result, err := commands.HandleCompress(m.ctx, text, m.ctxManager, m.messageCtx, m.ag.GetModel(), m.promptDir, "compact")
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/mcp") {
		result, err := commands.HandleMCP(text)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: result})
		}
		m.refreshView()
		return nil
	}

	if strings.HasPrefix(text, "/session") {
		result, err := m.handleSessionCommand(text)
		if err != nil {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: err.Error()})
		} else {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: result})
		}
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

func (m *AppModel) runAgent(input string) {
	if m.program == nil {
		return
	}
	_, err := m.ag.RunStream(m.ctx, m.messageCtx, input, func(token string) {
		m.program.Send(assistantTokenMsg{token: token})
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
			sb.WriteString(fmt.Sprintf("  %s%s - %s (updated: %s)\n", marker, s.ID, s.Title, s.UpdatedAt.Format("2006-01-02 15:04")))
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

	statusWidth := 30
	if m.width >= 120 {
		statusWidth = 34
	}
	if m.width < 80 {
		statusWidth = 24
	}
	m.statusWidth = statusWidth

	sidebarWidth := 24
	mainWidth := max(20, m.width-sidebarWidth-m.statusWidth-6)
	headerHeight := 3
	footerHeight := 1
	inputHeight := 4
	// 预留 2 字符给滚动条 + 2 字符内边距，防止内容被右侧面板遮挡
	m.viewport.Width = max(8, mainWidth-6)
	m.viewport.Height = max(1, m.height-headerHeight-footerHeight-inputHeight)
	m.input.SetWidth(max(8, m.viewport.Width-4))
	m.input.SetHeight(1)
}

func (m *AppModel) refreshView() {
	m.resize()
	contentWidth := max(8, m.viewport.Width)
	stickToBottom := m.autoScroll || m.viewport.AtBottom() || m.viewport.TotalLineCount() <= m.viewport.Height
	parts := make([]string, 0, len(m.entries))
	for _, entry := range m.entries {
		parts = append(parts, renderConversationEntry(entry, contentWidth))
	}
	m.viewport.SetContent(strings.Join(parts, "\n"))
	if stickToBottom {
		m.viewport.GotoBottom()
		m.autoScroll = true
	}
}

func (m *AppModel) snapshot() statusSnapshot {
	used, limit := 0, 0
	if m.ag != nil {
		used, limit = m.ag.TokenUsage()
	}
	snapshot := statusSnapshot{Busy: m.busy, CurrentState: animatedStateLabel(m.busy, m.currentStatus, m.spinnerFrame), TokenUsed: used, TokenLimit: limit, ToolCallsTotal: m.toolCalls, LastToolName: m.lastTool}
	if m.ctxManager != nil && m.messageCtx != nil {
		if messages, err := m.ctxManager.GetMessages(m.messageCtx); err == nil {
			snapshot.ContextMessages = len(messages)
			for _, msg := range messages {
				if strings.HasPrefix(msg.Content, "[对话历史摘要]") {
					snapshot.ContextSummaries++
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
		total, _, inProgress, _, _, _ := m.taskList.GetProgress()
		snapshot.TaskTotal = total
		snapshot.TaskInProgress = inProgress
		tasks := m.taskList.ListTasksByStatus(task.StatusInProgress)
		if len(tasks) == 0 {
			tasks = m.taskList.ListTasks()
		}
		for i, task := range tasks {
			if i >= 3 {
				break
			}
			snapshot.HighlightedTaskLine = append(snapshot.HighlightedTaskLine, fmt.Sprintf("%s %s", task.ID, task.Status))
		}
	}
	return snapshot
}

func renderSidebar(cursor int, height int) string {
	items := []string{
		titleStyle.MarginBottom(1).Render("NAVIGATOR"),
	}
	for i, item := range menuItems {
		icon := " "
		if i == 0 {
			icon = ""
		}
		label := fmt.Sprintf("%s %s", icon, item)
		if i == cursor {
			items = append(items, activeNavItemStyle.Render(label))
			continue
		}
		items = append(items, navItemStyle.Render(label))
	}
	items = append(items, "", lipgloss.NewStyle().Foreground(colorGreen).Render("● SYSTEM ONLINE"))
	return sidebarStyle.Height(max(1, height-1)).Render(lipgloss.JoinVertical(lipgloss.Left, items...))
}

func renderMainPane(m *AppModel) string {
	header := titleStyle.Foreground(colorBlue).Render(fmt.Sprintf("5HAGENT / %s", menuItems[max(0, min(len(menuItems)-1, m.sidebarCursor))]))
	stateLine := lipgloss.NewStyle().Foreground(colorGray).Render(fmt.Sprintf("model=%s | agent=%s | state=%s", fallback(m.modelName, "-"), m.agentName, animatedStateLabel(m.busy, m.currentStatus, m.spinnerFrame)))
	conversationHeight := max(1, m.viewport.Height)
	conversation := lipgloss.NewStyle().Height(conversationHeight).Render(renderViewportPane(m.viewport))
	inputBlock := inputShellStyle.Width(max(12, m.viewport.Width)).Render(m.input.View())
	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		stateLine,
		"",
		conversation,
		"",
		inputBlock,
	)
	return mainViewStyle.Width(max(20, m.width-24-m.statusWidth-4)).Height(max(1, m.height-1)).Render(content)
}

func renderBottomStatusBar(width int, status string, busy bool) string {
	state := fallback(status, "idle")
	if busy {
		state = animatedStateLabel(busy, status, 0)
	}
	parts := []string{"^C EXIT", "ENTER SEND", "PGUP/PGDN SCROLL", "STATE " + strings.ToUpper(state)}
	text := " " + strings.Join(parts, "  ")
	padding := width - lipgloss.Width(text)
	if padding < 0 {
		padding = 0
	}
	return statusBarStyle.Width(max(1, width)).Render(text + strings.Repeat(" ", padding))
}

func renderRow(label, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Foreground(colorGray).Width(8).Render(label),
		lipgloss.NewStyle().Foreground(colorText).Render(value))
}

func snapshotModelName(snapshot statusSnapshot) string {
	if snapshot.TokenLimit > 0 {
		return "ACTIVE"
	}
	return "STANDBY"
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

func compactToolEventText(event logger.ToolEvent) string {
	clean := strings.TrimSpace(stripANSI(event.Text))
	if clean == "" {
		return fmt.Sprintf("[tool] %s", fallback(tools.DisplayName(event.Name), event.Name))
	}

	entry := parseToolBlock(clean)
	displayName := fallback(entry.Name, tools.DisplayName(event.Name))
	displayName = fallback(displayName, event.Name)
	if displayName == "" {
		displayName = "tool"
	}

	switch event.Kind {
	case "call":
		parts := []string{fmt.Sprintf("[tool] %s", displayName)}
		if len(entry.Args) > 0 {
			parts = append(parts, strings.Join(entry.Args, " · "))
		}
		return truncateInline(strings.Join(parts, " "), 180)
	case "result":
		result := strings.Join(entry.Result, " · ")
		if result == "" {
			result = strings.TrimPrefix(clean, "⎿ ")
		}
		return truncateInline(fmt.Sprintf("[tool] %s done: %s", displayName, result), 180)
	case "error":
		message := fallback(entry.Error, clean)
		return truncateInline(fmt.Sprintf("[tool] %s error: %s", displayName, message), 180)
	default:
		return truncateInline("[tool] "+strings.ReplaceAll(clean, "\n", " · "), 180)
	}
}

func truncateInline(text string, maxLen int) string {
	text = strings.Join(strings.Fields(text), " ")
	return truncateMiddle(text, maxLen)
}

func renderConversationEntry(entry conversationEntry, width int) string {
	switch entry.Role {
	case roleUser:
		body := compactParagraph(strings.TrimSpace(entry.Content))
		return renderPrefixedPlainText("> ", body, colorPurple, width)
	case roleAssistant:
		content := strings.TrimRight(renderMarkdownForTerminal(normalizeAssistantContent(entry.Content), true), "\n")
		return wrapVisibleText(content, max(8, width))
	case roleHint:
		return renderHintEntry(entry.Content, width)
	case roleTool:
		return renderToolEntry(entry.Content, width)
	case roleSystem:
		return renderPrefixedPlainText("! ", strings.TrimSpace(entry.Content), colorBlue, width)
	default:
		return wrapVisibleText(strings.TrimSpace(entry.Content), width)
	}
}

func renderHintEntry(content string, width int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return renderPrefixedPlainText("  · ", "", colorGray, width)
	}
	lineWidth := max(8, width-lipgloss.Width("  · "))
	lines := wrapVisibleLines(content, lineWidth)
	for i, line := range lines {
		prefix := strings.Repeat(" ", lipgloss.Width("  · "))
		if i == 0 {
			prefix = lipgloss.NewStyle().Foreground(colorGray).Render("  · ")
		}
		lines[i] = prefix + colorizeToolHintLine(line)
	}
	return strings.Join(lines, "\n")
}

func colorizeToolHintLine(line string) string {
	plain := stripANSI(line)
	if !strings.HasPrefix(plain, "[tool] ") {
		return logger.Gray(line)
	}
	rest := strings.TrimPrefix(plain, "[tool] ")
	toolName := rest
	suffix := ""
	if idx := strings.IndexAny(rest, " \t:"); idx >= 0 {
		toolName = rest[:idx]
		suffix = rest[idx:]
	}
	return logger.Bold(logger.Blue("[tool]")) + " " + logger.Yellow(toolName) + logger.Gray(suffix)
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

func renderStatusPanel(snapshot statusSnapshot, width int) string {
	lines := []string{
		sectionTitleStyle.Render("SYSTEM_RESOURCES"),
		renderRow("STATE", snapshot.CurrentState),
		renderRow("BUSY", fmt.Sprintf("%v", snapshot.Busy)),
		renderRow("TOOLS", fmt.Sprintf("%d", snapshot.ToolCallsTotal)),
		renderRow("LAST", fallback(tools.DisplayName(snapshot.LastToolName), "-")),
		"",
		sectionTitleStyle.Render("ENVIRONMENT_CTX"),
		renderRow("MODEL", fallback(snapshotModelName(snapshot), "-")),
		renderRow("TOKENS", fmt.Sprintf("%d/%d", snapshot.TokenUsed, snapshot.TokenLimit)),
		renderRow("MSGS", fmt.Sprintf("%d", snapshot.ContextMessages)),
		renderRow("SUM", fmt.Sprintf("%d", snapshot.ContextSummaries)),
		"",
		sectionTitleStyle.Render("PROCESS_TREE"),
		renderRow("TASKS", fmt.Sprintf("%d", snapshot.TaskTotal)),
		renderRow("ACTIVE", fmt.Sprintf("%d", snapshot.TaskInProgress)),
		"",
		sectionTitleStyle.Render("SKILLS"),
	}
	if len(snapshot.EnabledSkills) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorGray).Render("(none)"))
	} else {
		for _, skill := range snapshot.EnabledSkills {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorGray).Render("• "+skill))
		}
	}
	if len(snapshot.HighlightedTaskLine) > 0 {
		lines = append(lines, "", sectionTitleStyle.Render("TASK_FOCUS"))
		for _, task := range snapshot.HighlightedTaskLine {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorGray).Render("• "+task))
		}
	} else {
		lines = append(lines, "", sectionTitleStyle.Render("TASK_FOCUS"), lipgloss.NewStyle().Foreground(colorGray).Render("• awaiting work"))
	}

	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapVisibleLines(line, width)...)
	}
	return strings.Join(wrapped, "\n")
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

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

func renderViewportPane(vp viewport.Model) string {
	contentLines := strings.Split(vp.View(), "\n")
	if len(contentLines) < vp.Height {
		for len(contentLines) < vp.Height {
			contentLines = append(contentLines, "")
		}
	}
	barLines := renderScrollbar(vp)
	rows := make([]string, 0, vp.Height)
	for i := 0; i < vp.Height; i++ {
		content := ""
		if i < len(contentLines) {
			content = contentLines[i]
		}
		bar := " "
		if i < len(barLines) {
			bar = barLines[i]
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, content, bar))
	}
	return strings.Join(rows, "\n")
}

func renderScrollbar(vp viewport.Model) []string {
	height := max(1, vp.Height)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = lipgloss.NewStyle().Foreground(colorGray).Render("│")
	}
	total := max(1, vp.TotalLineCount())
	if total <= height {
		for i := range lines {
			lines[i] = lipgloss.NewStyle().Foreground(colorGray).Render("┃")
		}
		return lines
	}
	thumbSize := max(1, height*height/total)
	maxOffset := max(1, total-height)
	thumbTop := (height - thumbSize) * vp.YOffset / maxOffset
	for i := thumbTop; i < min(height, thumbTop+thumbSize); i++ {
		lines[i] = lipgloss.NewStyle().Foreground(colorText).Render("┃")
	}
	return lines
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
	b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("TOOL " + strings.ToUpper(fallback(entry.Name, "event"))))

	if len(entry.Args) > 0 {
		joined := strings.Join(entry.Args, "   ·   ")
		wrapped := wrapVisibleLines(joined, max(8, width-2))
		b.WriteString("\n")
		for _, wl := range wrapped {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  " + wl))
			b.WriteString("\n")
		}
	}

	if entry.Error != "" {
		b.WriteString("\n")
		for _, wl := range wrapVisibleLines(entry.Error, max(8, width-2)) {
			b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Render("  " + wl))
			b.WriteString("\n")
		}
		return renderMessageBoxWithHeader(strings.TrimRight(b.String(), "\n"), width)
	}

	if len(entry.Result) > 0 {
		showLines := entry.Result
		truncated := false
		if len(showLines) > 2 {
			showLines = showLines[:2]
			truncated = true
		}
		b.WriteString("\n")
		for _, line := range showLines {
			wrapped := wrapVisibleLines(truncateMiddle(line, max(24, width+12)), max(8, width-2))
			for _, wl := range wrapped {
				b.WriteString(lipgloss.NewStyle().Foreground(colorText).Render("  " + wl))
				b.WriteString("\n")
			}
		}
		if truncated {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  ..."))
			b.WriteString("\n")
		}
	}
	return renderMessageBoxWithHeader(b.String(), width)
}

// renderToolUnknownEntry 渲染未知工具条目
func renderToolUnknownEntry(content string, width int) string {
	clean := strings.TrimSpace(ansiPattern.ReplaceAllString(content, ""))
	if clean == "" {
		return renderMessageBlock("TOOL_EXEC", "(empty)", colorYellow, width)
	}
	maxLen := width - 10
	if maxLen < 20 {
		maxLen = 20
	}
	return renderMessageBlock("TOOL_EXEC", truncateMiddle(clean, maxLen), colorYellow, width)
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
	if len(s) <= maxLen {
		return s
	}
	half := (maxLen - 3) / 2
	return s[:half] + "..." + s[len(s)-half:]
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

func LaunchTUI(ctx context.Context, ag *agent.Agent, modelName string, promptDir string, taskList *task.TaskList, skillMgr *skill.Manager, ctxManager *agentctx.Manager, messageCtx *agentctx.Context, sessionID string) error {
	launchMu.Lock()
	defer launchMu.Unlock()
	model := NewAppModel(ctx, ag, modelName, promptDir, taskList, skillMgr, ctxManager, messageCtx, sessionID)
	p := tea.NewProgram(model, tea.WithAltScreen())
	model.program = p
	logger.SetToolEventSink(func(event logger.ToolEvent) {
		p.Send(toolEventMsg{event: event})
	})
	defer logger.SetToolEventSink(nil)
	_, err := p.Run()
	return err
}
