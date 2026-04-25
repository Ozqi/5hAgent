package cli

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/commands"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/toolmeta"
)

const (
	roleUser         = "user"
	roleAssistant    = "assistant"
	roleTool         = "tool"
	roleSystem       = "system"
	defaultTUIWidth  = 100
	defaultTUIHeight = 30
)

type conversationEntry struct {
	Role    string
	Content string
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
	taskList   *agent.TaskList
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
	escPending       bool
	lastEscAt        time.Time
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

func NewAppModel(ctx context.Context, ag *agent.Agent, modelName string, taskList *agent.TaskList, skillMgr *skill.Manager, ctxManager *agentctx.Manager, messageCtx *agentctx.Context) *AppModel {
	vp := viewport.New(0, 0)

	input := textarea.New()
	input.Placeholder = "输入消息，Enter 发送"
	input.Focus()
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.Prompt = "> "
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	input.FocusedStyle.CursorLineNumber = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	input.BlurredStyle = input.FocusedStyle
	input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return &AppModel{
		ag:               ag,
		modelName:        modelName,
		agentName:        fallback(agentName(ag), "Agent"),
		taskList:         taskList,
		skillMgr:         skillMgr,
		ctxManager:       ctxManager,
		messageCtx:       messageCtx,
		ctx:              ctx,
		viewport:         vp,
		input:            input,
		currentAssistant: -1,
		currentStatus:    "idle",
	}
}

func (m *AppModel) SetProgram(p *tea.Program) {
	m.program = p
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
		if m.currentAssistant == -1 {
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
		if msg.event.Kind == "call" {
			m.toolCalls++
			m.lastTool = extractToolName(msg.event.Text)
		}
		m.entries = append(m.entries, conversationEntry{Role: roleTool, Content: strings.TrimRight(msg.event.Text, "\n")})
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
			m.viewport.ViewDown()
			return m, nil
		case "pgup", "ctrl+b":
			m.viewport.ViewUp()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.escPending = false
	m.input, cmd = m.input.Update(msg)
	m.viewport, _ = m.viewport.Update(msg)
	return m, cmd
}

func (m *AppModel) View() string {
	m.resize()
	conversationHeight := max(1, m.height-m.input.Height()-5)

	conversation := lipgloss.NewStyle().
		Width(max(1, m.width-m.statusWidth-1)).
		Height(conversationHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Render(m.viewport.View())

	status := lipgloss.NewStyle().
		Width(m.statusWidth).
		Height(conversationHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Render(renderStatusPanel(m.snapshot(), m.statusWidth-2))

	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, conversation, status)
	inputMeta := logger.Gray(fmt.Sprintf("model: %s | agent: %s", fallback(m.modelName, "-"), m.agentName))
	inputBox := lipgloss.NewStyle().
		Width(m.width).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Render(lipgloss.JoinVertical(lipgloss.Left, m.input.View(), inputMeta))

	return lipgloss.JoinVertical(lipgloss.Left, mainRow, inputBox)
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

	conversationWidth := max(20, m.width-m.statusWidth-4)
	conversationHeight := max(1, m.height-m.input.Height()-5)
	m.viewport.Width = conversationWidth
	m.viewport.Height = conversationHeight
	m.input.SetWidth(max(20, m.width-4))
	m.input.SetHeight(1)
}

func (m *AppModel) refreshView() {
	m.resize()
	parts := make([]string, 0, len(m.entries))
	for _, entry := range m.entries {
		parts = append(parts, renderConversationEntry(entry))
	}
	m.viewport.SetContent(strings.Join(parts, "\n"))
	m.viewport.GotoBottom()
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
		tasks := m.taskList.ListTasksByStatus(agent.StatusInProgress)
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

func renderConversationEntry(entry conversationEntry) string {
	switch entry.Role {
	case roleUser:
		return logger.Bold(logger.Green("You: ")) + compactParagraph(strings.TrimSpace(entry.Content))
	case roleAssistant:
		content := strings.TrimRight(renderMarkdownForTerminal(normalizeAssistantContent(entry.Content), true), "\n")
		return logger.Bold(logger.Cyan("Agent: ")) + content
	case roleTool:
		return renderToolEntry(entry.Content)
	case roleSystem:
		return logger.Gray("system: " + strings.TrimSpace(entry.Content))
	default:
		return strings.TrimSpace(entry.Content)
	}
}

func renderStatusPanel(snapshot statusSnapshot, width int) string {
	lines := []string{
		grayLabel("status"),
		grayValue(fmt.Sprintf("busy: %v", snapshot.Busy)),
		grayValue(fmt.Sprintf("state: %s", snapshot.CurrentState)),
		"",
		grayLabel("tokens"),
		grayValue(fmt.Sprintf("%d/%d", snapshot.TokenUsed, snapshot.TokenLimit)),
		"",
		grayLabel("context"),
		grayValue(fmt.Sprintf("messages: %d", snapshot.ContextMessages)),
		grayValue(fmt.Sprintf("summaries: %d", snapshot.ContextSummaries)),
		"",
		grayLabel("tools"),
		grayValue(fmt.Sprintf("calls: %d", snapshot.ToolCallsTotal)),
		grayValue(fmt.Sprintf("last: %s", fallback(toolmeta.DisplayName(snapshot.LastToolName), "-"))),
		"",
		grayLabel("skills"),
	}
	if len(snapshot.EnabledSkills) == 0 {
		lines = append(lines, grayValue("(none)"))
	} else {
		for _, skill := range snapshot.EnabledSkills {
			lines = append(lines, grayValue(skill))
		}
	}
	lines = append(lines,
		"",
		grayLabel("tasks"),
		grayValue(fmt.Sprintf("total: %d", snapshot.TaskTotal)),
		grayValue(fmt.Sprintf("in progress: %d", snapshot.TaskInProgress)),
	)
	if len(snapshot.HighlightedTaskLine) > 0 {
		for _, task := range snapshot.HighlightedTaskLine {
			lines = append(lines, grayValue(task))
		}
	}

	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if width <= 0 || len([]rune(line)) <= width || strings.TrimSpace(line) == "" {
		return []string{line}
	}
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{line}
	}
	lines := make([]string, 0)
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if len([]rune(candidate)) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current = candidate
	}
	lines = append(lines, current)
	return lines
}

func extractToolName(text string) string {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimPrefix(trimmed, "● ")
	if trimmed == "" {
		return ""
	}
	for i, r := range trimmed {
		if r == ' ' || r == '\n' || r == '[' {
			return trimmed[:i]
		}
	}
	return trimmed
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func renderToolEntry(content string) string {
	clean := strings.TrimSpace(ansiPattern.ReplaceAllString(content, ""))
	if clean == "" {
		return logger.Gray("tool")
	}
	lines := strings.Split(clean, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "● ")
		if i == 0 {
			lines[i] = logger.Gray("tool: " + line)
			continue
		}
		if line != "" {
			lines[i] = logger.Gray("  " + line)
		}
	}
	return strings.Join(lines, "\n")
}

func grayLabel(text string) string {
	return logger.Bold(logger.Gray(text))
}

func grayValue(text string) string {
	return logger.Gray(text)
}

func agentName(ag *agent.Agent) string {
	if ag == nil {
		return ""
	}
	return ag.Name()
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

func LaunchTUI(ctx context.Context, ag *agent.Agent, modelName string, taskList *agent.TaskList, skillMgr *skill.Manager, ctxManager *agentctx.Manager, messageCtx *agentctx.Context) error {
	launchMu.Lock()
	defer launchMu.Unlock()
	model := NewAppModel(ctx, ag, modelName, taskList, skillMgr, ctxManager, messageCtx)
	p := tea.NewProgram(model, tea.WithAltScreen())
	model.SetProgram(p)
	prevOutput := logger.Output()
	logger.SetToolEventSink(func(event logger.ToolEvent) {
		p.Send(toolEventMsg{event: event})
	})
	defer logger.SetToolEventSink(nil)
	logger.SetOutput(io.Discard)
	defer logger.SetOutput(prevOutput)
	_, err := p.Run()
	return err
}
