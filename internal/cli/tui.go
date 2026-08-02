// tui.go - 终端 UI 主界面
// 功能：Bubble Tea 构建的交互式对话界面，显示对话/状态面板，支持 /task /skill /compress 命令
// 主要类型：AppModel, conversationEntry, statusSnapshot
// 导出函数：NewAppModel, LaunchTUI
package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ansi "github.com/charmbracelet/x/ansi"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/tools"
	"github.com/mattn/go-runewidth"
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
	quitConfirmDelay = 2 * time.Second
)

type conversationEntry struct {
	Role        string
	Content     string
	CreatedAt   string
	ToolName    string
	ToolArgs    string
	ToolKey     string
	ToolState   string
	ToolOutput  string
	SystemTitle string
}

func entryNow(entry conversationEntry) conversationEntry {
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return entry
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
	ContextTokens    int
	ContextWindow    int
	SessionTokens    int
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
	runCancel   context.CancelFunc
	runEntry    int
	runLines    []string

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
	lastInput        string
	escPending       bool
	lastEscAt        time.Time
	quitPending      bool
	lastQuitAt       time.Time
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

// ToolEventFunc 接收 TUI 普通对话中的工具事件。
type ToolEventFunc func(logger.ToolEvent)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var launchMu sync.Mutex

var (
	colorGreen   = lipgloss.Color("#9ece6a")
	colorBlue    = lipgloss.Color("#7aa2f7")
	colorPurple  = lipgloss.Color("#bb9af7")
	colorYellow  = lipgloss.Color("#e0af68")
	colorGray    = lipgloss.Color("#565f89")
	colorWhite   = lipgloss.Color("#e2e1f1")
	colorCommand = lipgloss.Color("#89b4fa")
	colorResult  = lipgloss.Color("#cdd6f4")
	colorError   = lipgloss.Color("#f38ba8")
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
)

type slashCommandHint struct {
	Name  string
	Usage string
	Desc  string
}

var slashCommandHints = []slashCommandHint{
	{Name: "/task", Usage: "/task <list|create|update|get|delete|archive|reopen>", Desc: "task file"},
	{Name: "/skill", Usage: "/skill <list|get|reload>", Desc: "skills"},
	{Name: "/compress", Usage: "/compress", Desc: "context"},
	{Name: "/mcp", Usage: "/mcp <list|add|remove|enable|disable>", Desc: "mcp servers"},
	{Name: "/session", Usage: "/session <new|list|id>", Desc: "sessions"},
	{Name: "/run", Usage: "/run", Desc: "run task.md until no pending tasks"},
	{Name: "/stop", Usage: "/stop", Desc: "stop current run"},
	{Name: "/detach", Usage: "/detach", Desc: "detach tmux client"},
	{Name: "/model", Usage: "/model <provider/model>", Desc: "ollama/gemma4, mira/gpt-5.5"},
}

var modelHints = []string{
	"ollama/gemma4",
	"ollama/qwen3:14b",
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
		runEntry:         -1,
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
	toolEntries := make(map[string]int)
	for _, msg := range messages {
		var r string
		switch msg.Role {
		case schema.User:
			r = roleUser
		case schema.Assistant:
			if msg.ReasoningContent != "" {
				entries = append(entries, conversationEntry{Role: roleThinking, Content: msg.ReasoningContent, CreatedAt: messageCreatedAt(msg)})
			}
			for _, tc := range msg.ToolCalls {
				name := fallback(tools.DisplayName(tc.Function.Name), tc.Function.Name)
				entry := conversationEntry{
					Role:      roleHint,
					CreatedAt: messageCreatedAt(msg),
					ToolName:  name,
					ToolArgs:  formatToolArgsSummary(tc.Function.Arguments),
					ToolKey:   toolEventKey(tc.Function.Name, tc.Function.Arguments),
					ToolState: "done",
				}
				entries = append(entries, entry)
				if tc.ID != "" {
					toolEntries[tc.ID] = len(entries) - 1
				}
			}
			if msg.Content == "" {
				continue
			}
			r = roleAssistant
		case schema.System:
			r = roleSystem
		case schema.Tool:
			output := compactOutputLines(strings.Split(msg.Content, "\n"), 4)
			if idx, ok := toolEntries[msg.ToolCallID]; ok {
				entries[idx].ToolOutput = output
				continue
			}
			entries = append(entries, conversationEntry{Role: roleHint, CreatedAt: messageCreatedAt(msg), ToolName: fallback(msg.ToolName, "tool"), ToolState: "done", ToolOutput: output})
			continue
		default:
			continue
		}
		entries = append(entries, conversationEntry{Role: r, Content: msg.Content, CreatedAt: messageCreatedAt(msg)})
	}
	return entries
}

func messageCreatedAt(msg *schema.Message) string {
	if msg == nil || msg.Extra == nil {
		return ""
	}
	if v, ok := msg.Extra["created_at"].(string); ok {
		return v
	}
	return ""
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
		if !m.busy {
			return m, nil
		}
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
		if !m.busy {
			return m, nil
		}
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
			m.finishRunEntry(content)
		} else {
			m.currentStatus = "idle"
			m.finishRunEntry(msg.summary)
		}
		m.refreshView()
		return m, nil
	case toolEventMsg:
		if msg.event.Kind == "call" {
			m.toolCalls++
			m.lastTool = fallback(tools.DisplayName(msg.event.Name), msg.event.Name)
		}
		if m.runEntry >= 0 {
			m.applyRunToolEvent(msg.event)
		} else {
			m.applyToolEvent(msg.event)
		}
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
		// tmux mouse 转义序列偶尔会以普通按键漏进来；这里直接吞掉，
		// 避免 `[<65;...M` 之类的滚轮事件污染输入框。
		if isMouseEscapeKey(msg.String()) {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			if confirm(&m.quitPending, &m.lastQuitAt, quitConfirmDelay) {
				return m, tea.Quit
			}
			m.currentStatus = "ctrl+c again to quit"
			m.refreshView()
			return m, nil
		case "ctrl+u":
			m.input.Reset()
			m.refreshView()
			return m, nil
		case "ctrl+d":
			return m, tea.Quit
		case "esc":
			if confirm(&m.escPending, &m.lastEscAt, quitConfirmDelay) {
				return m, tea.Quit
			}
			m.currentStatus = "esc again to quit"
			m.refreshView()
			return m, nil
		case "enter":
			if msg.Paste {
				break
			}
			return m, m.submit()
		case "tab":
			text := strings.TrimSpace(m.input.Value())
			if strings.HasPrefix(text, "/") && !strings.Contains(text, " ") {
				if matches := slashHintMatches(text); len(matches) == 1 {
					m.input.SetValue(matches[0].Name + " ")
				}
			}
			return m, nil
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
		case "up":
			// 只保留最近一次提交，满足快速重复输入；多级 shell history 暂不引入。
			if m.lastInput != "" {
				m.input.SetValue(m.lastInput)
			}
			return m, nil
		case "ctrl+p":
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
	m.input, cmd = m.input.Update(msg)
	if _, ok := msg.(tea.KeyMsg); ok {
		m.escPending = false
		m.quitPending = false
	}
	m.viewport, _ = m.viewport.Update(msg)
	m.autoScroll = m.viewport.AtBottom()
	return m, cmd
}

func isMouseEscapeKey(text string) bool {
	return strings.HasPrefix(text, "\x1b[<") || strings.HasPrefix(text, "[<")
}

// confirm 处理“短时间内二次按键确认”的通用状态。
func confirm(pending *bool, last *time.Time, within time.Duration) bool {
	now := time.Now()
	if *pending && now.Sub(*last) <= within {
		*pending = false
		return true
	}
	*pending = true
	*last = now
	return false
}

func (m *AppModel) View() string {
	m.resize()
	// 当前布局保持单列：历史记录在上，输入框附近承载运行状态。
	mainView := renderMainPane(m)
	statusBar := renderBottomStatusBar(m.width)
	mainHeight := max(1, m.height-1)
	return lipgloss.JoinVertical(lipgloss.Left,
		renderFixedLines(strings.Split(mainView, "\n"), max(1, m.width), mainHeight),
		statusBar,
	)
}

func (m *AppModel) resize() {
	if m.width <= 0 {
		m.width = defaultTUIWidth
	}
	if m.height <= 0 {
		m.height = defaultTUIHeight
	}

	mainWidth := max(20, m.width)
	m.input.SetWidth(max(8, mainWidth-2))
	m.input.SetHeight(1)
	m.viewport.Width = max(8, mainWidth)
	m.viewport.Height = max(1, m.height-1-m.reservedMainHeight(mainWidth))
}

// reservedMainHeight 返回 viewport 之外的主界面行数。
// 这里按真实渲染文本计数，避免窄屏 slash hint 或长输入换行后挤到输入框下方。
func (m *AppModel) reservedMainHeight(width int) int {
	headerHeight := 1
	inputHeight := renderedLineCount(renderInputBar(m.input.View(), width))
	footerHeight := 1
	slashHeight := renderedLineCount(m.renderSlashHint(max(12, width-4)))
	return headerHeight + slashHeight + inputHeight + footerHeight
}

func renderedLineCount(text string) int {
	if strings.TrimSpace(stripANSI(text)) == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func (m *AppModel) refreshView() {
	m.resize()
	contentWidth := max(8, m.viewport.Width)
	stickToBottom := m.autoScroll || m.viewport.AtBottom() || m.viewport.TotalLineCount() <= m.viewport.Height
	parts := make([]string, 0, len(m.entries))
	for i := range m.entries {
		if m.entries[i].CreatedAt == "" {
			m.entries[i] = entryNow(m.entries[i])
		}
		entry := m.entries[i]
		parts = append(parts, m.renderConversationEntry(entry, contentWidth))
	}
	if len(parts) == 0 {
		parts = append(parts, renderEmptyState(contentWidth))
	}
	m.viewText = strings.Join(parts, "\n\n")
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
	contextTokens, sessionTokens, contextWindow := 0, 0, 0
	turn := 0
	if m.ag != nil {
		contextTokens, sessionTokens, contextWindow = m.ag.TokenUsage()
		turn = m.ag.CurrentTurn()
	}
	snapshot := statusSnapshot{Runtime: runtimeMeta{
		Busy:           m.busy,
		State:          animatedStateLabel(m.busy, m.currentStatus, m.spinnerFrame),
		Turn:           turn,
		ContextTokens:  contextTokens,
		ContextWindow:  contextWindow,
		SessionTokens:  sessionTokens,
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

func renderBottomStatusBar(width int) string {
	return statusBarStyle.Width(max(1, width)).Render(strings.Repeat(" ", max(1, width)))
}

func (m *AppModel) renderConversationEntry(entry conversationEntry, width int) string {
	innerWidth := max(8, width-2)
	switch entry.Role {
	case roleUser:
		body := compactParagraph(strings.TrimSpace(entry.Content))
		return withEntryTime(entry, renderUserEntry(body, width), width)
	case roleAssistant:
		content := strings.TrimRight(renderMarkdownForTerminal(normalizeAssistantContent(entry.Content), true), "\n")
		return renderIndentedEntry(withEntryTime(entry, wrapVisibleText(content, innerWidth), innerWidth))
	case roleHint:
		return renderIndentedEntry(withEntryTime(entry, m.renderToolHintEntry(entry, innerWidth), innerWidth))
	case roleThinking:
		return renderIndentedEntry(withEntryTime(entry, renderThinkingEntry(entry.Content, innerWidth), innerWidth))
	case roleTool:
		return renderIndentedEntry(withEntryTime(entry, renderToolEntry(entry.Content, innerWidth), innerWidth))
	case roleSystem:
		return renderIndentedEntry(withEntryTime(entry, renderSystemEntry(entry.SystemTitle, entry.Content, innerWidth), innerWidth))
	default:
		return renderIndentedEntry(withEntryTime(entry, wrapVisibleText(strings.TrimSpace(entry.Content), innerWidth), innerWidth))
	}
}

func renderIndentedEntry(rendered string) string {
	return indentLines(rendered, "  ", "  ")
}

func withEntryTime(entry conversationEntry, rendered string, width int) string {
	label := entryTimeLabel(entry.CreatedAt)
	if label == "" || strings.TrimSpace(stripANSI(rendered)) == "" {
		return rendered
	}
	return appendRightLabel(rendered, label, width)
}

func entryTimeLabel(createdAt string) string {
	if createdAt == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		return t.Local().Format("15:04:05")
	}
	return createdAt
}

func appendRightLabel(rendered string, label string, width int) string {
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	styled := lipgloss.NewStyle().Foreground(colorMuted).Faint(true).Render(label)
	width = max(12, width)
	if lipgloss.Width(lines[0])+lipgloss.Width(styled)+2 <= width {
		padding := width - lipgloss.Width(lines[0]) - lipgloss.Width(styled)
		lines[0] += strings.Repeat(" ", max(2, padding)) + styled
		return strings.Join(lines, "\n")
	}
	lines[0] += " " + styled
	return strings.Join(lines, "\n")
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
		line = clipVisibleLine(line, width)
		padding := width - lipgloss.Width(line)
		if padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		fixed = append(fixed, line)
	}
	return strings.Join(fixed, "\n")
}

func clipVisibleLine(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}
	var b strings.Builder
	visible := 0
	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			if end := strings.IndexByte(line[i:], 'm'); end >= 0 {
				b.WriteString(line[i : i+end+1])
				i += end + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		rw := runewidth.RuneWidth(r)
		if visible+rw > width {
			break
		}
		b.WriteRune(r)
		visible += rw
		i += size
	}
	return b.String()
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
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

func LaunchTUI(ctx context.Context, ag *agent.Agent, modelName string, promptDir string, taskList *task.TaskList, skillMgr *skill.Manager, ctxManager *agentctx.Manager, messageCtx *agentctx.Context, sessionID string, runTasks RunTasksFunc, switchModel SwitchModelFunc, onToolEvent ToolEventFunc) error {
	launchMu.Lock()
	defer launchMu.Unlock()
	model := NewAppModel(ctx, ag, modelName, promptDir, taskList, skillMgr, ctxManager, messageCtx, sessionID, runTasks, switchModel)
	// WithMouseCellMotion 开启点击、释放和滚轮事件；viewport.Update 负责具体滚动。
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	model.program = p
	prevSink := ag.SetToolEventSink(func(event logger.ToolEvent) {
		if onToolEvent != nil {
			onToolEvent(event)
		}
		p.Send(toolEventMsg{event: event})
	})
	defer ag.SetToolEventSink(prevSink)
	_, err := p.Run()
	return err
}
