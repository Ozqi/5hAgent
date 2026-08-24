// Package tui 实现基于 Bubble Tea 的终端对话界面、daemon attach 界面和运行状态渲染。
package tui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Ozqi/walle/internal/systemd"
	"github.com/Ozqi/walle/internal/toolevent"
	"github.com/Ozqi/walle/internal/tools"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

const (
	roleUser         = "user"
	roleAssistant    = "assistant"
	roleSystem       = "system"
	roleHint         = "hint"
	roleThinking     = "thinking"
	defaultTUIWidth  = 100
	defaultTUIHeight = 30
	quitConfirmDelay = 2 * time.Second
	maxHintRows      = 12
)

type conversationEntry struct {
	Role        string
	Content     string
	CreatedAt   string
	ToolName    string
	ToolIntent  string
	ToolArgs    string
	ToolKey     string
	ToolState   string
	ToolOutput  string
	SystemTitle string

	renderCacheKey   string
	renderCacheWidth int
	renderCacheFrame int
	renderCacheText  string
}

func entryNow(entry conversationEntry) conversationEntry {
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return entry
}

type statusSnapshot struct {
	Runtime runtimeMeta
}

type runtimeMeta struct {
	Busy           bool
	State          string
	Turn           int
	ScrollPercent  int
	ToolCallsTotal int
	LastToolName   string
	PendingInput   bool
	SessionID      string
	Workdir        string
	Git            gitMeta
}

type gitMeta struct {
	Repo      bool
	Worktree  bool
	Branch    string
	Dirty     bool
	Shortstat string
}

// AppModel 保存 attached TUI 当前帧所需的全部状态。
// 调用层级：LaunchAttachedTUI -> NewAppModel -> Bubble Tea Update/View。
// 设计边界：UI 只持有 remote client 回调和渲染快照，不直接持有 Runtime 或 Agent。
type AppModel struct {
	program      *tea.Program
	modelName    string
	sessionID    string
	remoteSubmit func(string) error
	remoteStop   func() error
	ctx          context.Context

	width  int
	height int
	busy   bool

	viewport  viewport.Model
	input     textarea.Model
	entries   []conversationEntry
	viewText  string
	toolCalls int
	lastTool  string

	currentAssistant   int
	currentStatus      string
	remoteTurn         int
	spinnerFrame       int
	spinnerPending     bool
	renderPending      bool
	remoteDisconnected bool
	lastInput          string
	pendingInput       string
	escPending         bool
	lastEscAt          time.Time
	quitPending        bool
	lastQuitAt         time.Time
	autoScroll         bool
	metaCache          cachedMeta
	picker             *pickerState
}

type pickerState struct {
	Kind     string
	Provider string
	Options  []string
	Cursor   int
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
	event toolevent.ToolEvent
}

type remoteEventMsg struct{ event systemd.ProcessEvent }

type remoteDisconnectedMsg struct{}

type spinnerTickMsg struct{}

type renderTickMsg struct{}

type locationLoadedMsg struct {
	workdir string
	git     gitMeta
}

type remoteSubmitResultMsg struct {
	text string
	err  error
}

type remoteStopResultMsg struct {
	err error
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var (
	colorGreen   = lipgloss.Color("#9ece6a")
	colorBlue    = lipgloss.Color("#7aa2f7")
	colorPurple  = lipgloss.Color("#bb9af7")
	colorOrange  = lipgloss.Color("#DFA241")
	colorYellow  = lipgloss.Color("#F2C14E")
	colorGray    = lipgloss.Color("#565f89")
	colorWhite   = lipgloss.Color("#e2e1f1")
	colorCommand = colorOrange
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
	{Name: "/skill", Usage: "/skill <list|get|reload>", Desc: "skills"},
	{Name: "/compress", Usage: "/compress", Desc: "context"},
	{Name: "/mcp", Usage: "/mcp <list|add|remove|enable|disable>", Desc: "mcp servers"},
	{Name: "/session", Usage: "/session <new|list|id>", Desc: "sessions"},
	{Name: "/stop", Usage: "/stop", Desc: "stop current run"},
	{Name: "/model", Usage: "/model <provider/model>", Desc: "ollama/gemma4, mira/gpt-5.5"},
	{Name: "/provider", Usage: "/provider [name]", Desc: "select and authenticate provider"},
}

var modelHints = []string{
	"ollama/gemma4",
	"ollama/qwen3:14b",
	"mira/gpt-5.4",
	"mira/gpt-5.5",
	"mira/glm-5.2",
	"mira/claude-opus-4-6",
}

// NewAppModel 创建 attached TUI 的初始模型。
func NewAppModel(ctx context.Context, modelName string, sessionID string) *AppModel {
	vp := viewport.New(0, 0)
	// viewport 自身支持滚轮，但还需要 LaunchAttachedTUI 开启 Bubble Tea mouse mode。
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 2

	input := textarea.New()
	input.Placeholder = ""
	input.Focus()
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.Prompt = "> "
	// 输入框使用参考 tmux 对话窗口的低对比深灰条，避免大白块抢视觉焦点。
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorYellow).Background(colorInputBg).Bold(true)
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorInputFg).Background(colorInputBg)
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorGray).Background(colorInputBg)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(colorInputFg).Background(colorInputBg)
	input.FocusedStyle.CursorLineNumber = lipgloss.NewStyle().Foreground(colorGray).Background(colorInputBg)
	input.BlurredStyle = input.FocusedStyle

	return &AppModel{
		modelName:        modelName,
		sessionID:        sessionID,
		ctx:              ctx,
		viewport:         vp,
		input:            input,
		currentAssistant: -1,
		currentStatus:    "idle",
		autoScroll:       true,
	}
}

// Init 返回 Bubble Tea 启动时需要执行的光标闪烁和异步元信息加载命令。
func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.loadRuntimeLocationCmd())
}

// Update 处理 Bubble Tea 消息并更新 TUI 状态机。
// 消息按布局、本地 Agent 输出、daemon attach 事件、工具事件、picker 模态和键鼠输入分层处理。
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// 布局事件只更新尺寸和缓存视图，不触发 Agent 状态变化。
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.refreshView()
		return m, nil
	case assistantTokenMsg:
		// 高频 token 只改状态并排队渲染，避免每个 chunk 全量重绘历史。
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
		return m, tea.Batch(m.queueRender(), m.queueSpinner())
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
		return m, tea.Batch(m.queueRender(), m.queueSpinner())
	case assistantDoneMsg:
		m.busy = false
		m.currentAssistant = -1
		m.currentStatus = "idle"
		m.renderPending = false
		m.refreshView()
		return m, tea.Batch(m.loadRuntimeLocationCmd(), m.submitPendingInputCmd())
	case assistantErrorMsg:
		m.busy = false
		m.currentAssistant = -1
		m.currentStatus = "error"
		m.renderPending = false
		m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: "agent error: " + msg.err.Error()})
		m.refreshView()
		return m, tea.Batch(m.loadRuntimeLocationCmd(), m.submitPendingInputCmd())
	case remoteEventMsg:
		// attached 模式把 daemon 协议事件翻译成本地 TUI 状态，不直接访问 Runtime。
		event := msg.event
		if event.Turn > 0 {
			m.remoteTurn = event.Turn
		}
		switch event.Type {
		case systemd.ProcessEventUser:
			m.entries = append(m.entries, conversationEntry{Role: roleUser, Content: event.Text})
			m.refreshView()
			return m, nil
		case systemd.ProcessEventState:
			m.busy = event.Busy
			if event.Busy {
				m.currentStatus = "running"
				m.refreshView()
				return m, m.queueSpinner()
			}
			m.currentStatus = "idle"
			m.refreshView()
			return m, m.submitPendingInputCmd()
		case systemd.ProcessEventAssistant:
			return m.Update(assistantTokenMsg{token: event.Text})
		case systemd.ProcessEventThinking:
			return m.Update(assistantThinkingMsg{token: event.Text})
		case systemd.ProcessEventTool:
			return m.Update(toolEventMsg{event: toolevent.ToolEvent{Kind: event.Kind, Name: event.Name, Args: event.Args, Text: event.Text, Result: event.Result, Error: event.Error}})
		case systemd.ProcessEventSystem:
			m.busy = false
			m.currentStatus = "idle"
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: event.Text})
			m.refreshView()
			return m, nil
		case systemd.ProcessEventPicker:
			m.busy = false
			m.currentStatus = "select " + event.Kind
			m.picker = &pickerState{Kind: event.Kind, Provider: event.Name, Options: append([]string(nil), event.Options...)}
			m.refreshView()
			return m, nil
		case systemd.ProcessEventModel:
			m.busy = false
			m.modelName = event.Text
			m.currentStatus = "idle"
			m.refreshView()
			return m, nil
		case systemd.ProcessEventDone:
			return m.Update(assistantDoneMsg{})
		case systemd.ProcessEventError:
			return m.Update(assistantErrorMsg{err: fmt.Errorf("%s", event.Error)})
		}
		return m, nil
	case remoteDisconnectedMsg:
		if m.remoteDisconnected {
			return m, nil
		}
		m.remoteDisconnected = true
		m.busy = false
		m.currentStatus = "disconnected"
		m.remoteSubmit = func(string) error { return fmt.Errorf("daemon disconnected") }
		m.remoteStop = func() error { return fmt.Errorf("daemon disconnected") }
		if len(m.entries) == 0 || m.entries[len(m.entries)-1].Content != "daemon disconnected" {
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: "daemon disconnected"})
		}
		m.renderPending = false
		m.refreshView()
		return m, nil
	case spinnerTickMsg:
		// spinner 只保留一个定时链，避免 token 密集时堆积大量 Tick。
		m.spinnerPending = false
		if m.busy {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			m.refreshView()
			return m, m.queueSpinner()
		}
		return m, nil
	case renderTickMsg:
		m.renderPending = false
		m.refreshView()
		return m, nil
	case locationLoadedMsg:
		m.metaCache = cachedMeta{Workdir: msg.workdir, Git: msg.git, LoadedAt: time.Now()}
		m.refreshView()
		return m, nil
	case remoteSubmitResultMsg:
		if msg.err != nil {
			m.busy = false
			m.currentStatus = "error"
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: msg.err.Error()})
			m.refreshView()
			return m, nil
		}
		if strings.HasPrefix(strings.TrimSpace(msg.text), "/") {
			m.currentStatus = "idle"
			m.refreshView()
		}
		return m, nil
	case remoteStopResultMsg:
		if msg.err != nil {
			m.currentStatus = "error"
			m.entries = append(m.entries, conversationEntry{Role: roleSystem, Content: msg.err.Error()})
			m.refreshView()
		}
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
			return m, m.queueSpinner()
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
			m.input.Reset()
			m.picker = nil
			m.currentStatus = "input cleared; ctrl+c again to quit"
			m.refreshView()
			return m, nil
		case "ctrl+d":
			return m, tea.Quit
		}
		if m.picker != nil {
			// picker 是模态输入；存在时不让普通快捷键和 textarea 继续消费按键。
			switch msg.String() {
			case "up", "ctrl+p":
				if m.picker.Cursor > 0 {
					m.picker.Cursor--
				}
				m.refreshView()
				return m, nil
			case "down", "ctrl+n":
				if m.picker.Cursor+1 < len(m.picker.Options) {
					m.picker.Cursor++
				}
				m.refreshView()
				return m, nil
			case "esc":
				m.picker = nil
				m.currentStatus = "idle"
				m.refreshView()
				return m, nil
			case "enter":
				if len(m.picker.Options) == 0 || m.remoteSubmit == nil {
					return m, nil
				}
				value := m.picker.Options[m.picker.Cursor]
				command := "/" + m.picker.Kind + " " + value
				if m.picker.Kind == "model" {
					command = "/model " + m.picker.Provider + "/" + value
				}
				m.picker = nil
				m.currentStatus = "command"
				m.refreshView()
				return m, remoteSubmitCmd(m.remoteSubmit, command)
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+u":
			m.input.Reset()
			m.quitPending = false
			m.refreshView()
			return m, nil
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
			raw := m.input.Value()
			text := strings.TrimSpace(raw)
			if text == "/model" {
				m.input.SetValue("/model ")
				m.input.CursorEnd()
				m.refreshView()
				return m, nil
			}
			if isModelHintInput(raw, text) {
				matches := m.modelHintMatches(modelArgPrefix(raw))
				if len(matches) == 1 {
					m.input.SetValue("/model " + matches[0])
					m.input.CursorEnd()
				}
				m.refreshView()
				return m, nil
			}
			if strings.HasPrefix(text, "/") && !strings.Contains(text, " ") {
				if matches := slashHintMatches(text); len(matches) == 1 {
					m.input.SetValue(matches[0].Name + " ")
					m.input.CursorEnd()
				}
				m.refreshView()
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
		if m.cleanInputValue() {
			m.refreshView()
		}
	}
	m.viewport, _ = m.viewport.Update(msg)
	m.autoScroll = m.viewport.AtBottom()
	return m, cmd
}

func (m *AppModel) submitPendingInputCmd() tea.Cmd {
	if m.busy || m.remoteSubmit == nil || strings.TrimSpace(m.pendingInput) == "" {
		return nil
	}
	text := m.pendingInput
	m.pendingInput = ""
	m.busy = true
	m.currentStatus = "submitting queued"
	m.refreshView()
	return tea.Batch(remoteSubmitCmd(m.remoteSubmit, text), m.queueSpinner())
}

func (m *AppModel) queueSpinner() tea.Cmd {
	if !m.busy || m.spinnerPending {
		return nil
	}
	m.spinnerPending = true
	return tickSpinner()
}

func (m *AppModel) queueRender() tea.Cmd {
	if m.renderPending {
		return nil
	}
	m.renderPending = true
	return tea.Tick(33*time.Millisecond, func(time.Time) tea.Msg { return renderTickMsg{} })
}

func (m *AppModel) loadRuntimeLocationCmd() tea.Cmd {
	workdir := m.metaCache.Workdir
	if workdir == "" || workdir == "-" {
		if cwd, err := os.Getwd(); err == nil {
			workdir = cwd
		}
	}
	if workdir == "" || workdir == "-" {
		return nil
	}
	return func() tea.Msg {
		return locationLoadedMsg{workdir: workdir, git: readGitMeta(workdir)}
	}
}

var mouseEscapePattern = regexp.MustCompile(`(?:\x1b)?\[<[0-9;]*[mM]?`)

func isMouseEscapeKey(text string) bool {
	return mouseEscapePattern.MatchString(text)
}

func stripMouseEscapeSequences(text string) string {
	return mouseEscapePattern.ReplaceAllString(text, "")
}

func (m *AppModel) cleanInputValue() bool {
	value := m.input.Value()
	cleaned := stripMouseEscapeSequences(value)
	if cleaned == value {
		return false
	}
	m.input.SetValue(cleaned)
	m.input.CursorEnd()
	return true
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

// View 渲染当前 TUI 画面，并保留底部占位行避免输入框贴边。
func (m *AppModel) View() string {
	m.resize()
	// 当前布局保持单列：历史记录在上，输入框附近承载运行状态。
	mainView := renderMainPane(m)
	mainHeight := max(1, m.height-1)
	bottomPad := statusBarStyle.Width(max(1, m.width)).Render(strings.Repeat(" ", max(1, m.width)))
	return lipgloss.JoinVertical(lipgloss.Left,
		renderFixedLines(strings.Split(mainView, "\n"), max(1, m.width), mainHeight),
		bottomPad,
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
	footerHeight := renderedLineCount(renderInputFooter(m.snapshot(), m.sessionID, width))
	slashHeight := renderedLineCount(m.renderSlashHint(max(12, width-4)))
	return headerHeight + slashHeight + inputHeight + footerHeight
}

func renderedLineCount(text string) int {
	if strings.TrimSpace(stripANSI(text)) == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// refreshView 重新渲染所有会话条目，并在 auto-scroll 开启时保持贴底。
func (m *AppModel) refreshView() {
	m.resize()
	contentWidth := max(8, m.viewport.Width)
	stickToBottom := m.autoScroll || m.viewport.AtBottom() || m.viewport.TotalLineCount() <= m.viewport.Height
	parts := make([]string, 0, len(m.entries))
	for i := range m.entries {
		if m.entries[i].CreatedAt == "" {
			m.entries[i] = entryNow(m.entries[i])
		}
		parts = append(parts, m.renderConversationEntryCached(i, contentWidth))
	}
	if len(parts) == 0 {
		parts = append(parts, renderEmptyState(contentWidth, m.viewport.Height))
	}
	m.viewText = strings.Join(parts, "\n\n")
	m.viewport.SetContent(m.viewText)
	if stickToBottom {
		m.viewport.GotoBottom()
		m.autoScroll = true
	}
}

func renderEmptyState(width int, height int) string {
	padBottom := func(content string) string {
		return content + strings.Repeat("\n", max(0, height-renderedLineCount(content)))
	}
	if width >= 40 && height >= 8 {
		info := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Render("walle"),
			lipgloss.NewStyle().Foreground(colorMuted).Render("Go Agent Runtime"),
			lipgloss.NewStyle().Foreground(colorMuted).Render("Inspect · Patch · Run"),
			lipgloss.NewStyle().Foreground(colorMuted).Render("type / for commands"),
		)
		return padBottom(lipgloss.NewStyle().PaddingLeft(1).Render(lipgloss.JoinHorizontal(
			lipgloss.Top,
			renderWallePixelIcon(),
			"   ",
			info,
		)))
	}
	if width >= 16 && height >= 8 {
		return padBottom(lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(renderWallePixelIcon()))
	}
	if width >= 12 && height >= 4 {
		return padBottom(lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(renderWallePixelIconCompact()))
	}
	lines := []string{
		lipgloss.NewStyle().Foreground(colorYellow).Render("walle 已就绪"),
		lipgloss.NewStyle().Foreground(colorMuted).Render("type a prompt to start"),
		lipgloss.NewStyle().Foreground(colorMuted).Render("type / for commands"),
	}
	for i, line := range lines {
		lines[i] = truncateMiddle(line, max(20, width-2))
	}
	return padBottom(strings.Join(lines, "\n"))
}

func renderWallePixelIcon() string {
	orange := lipgloss.NewStyle().Foreground(colorOrange)
	yellow := lipgloss.NewStyle().Foreground(colorYellow)
	return strings.Join([]string{
		orange.Render(" ╭───╮ ╭───╮"),
		orange.Render("╱  ") + yellow.Render("●") + orange.Render(" ╲_╱ ") + yellow.Render("●") + orange.Render("  ╲"),
		orange.Render("╲____╱ ╲____╱"),
		orange.Render("     ║╬║"),
		orange.Render("╭██╮╭─╨─╮╭██╮"),
		orange.Render("│██├┤") + yellow.Render("▪▦▪") + orange.Render("├┤██│"),
		orange.Render("╰██╯╰───╯╰██╯"),
	}, "\n")
}

func renderWallePixelIconCompact() string {
	orange := lipgloss.NewStyle().Foreground(colorOrange)
	yellow := lipgloss.NewStyle().Foreground(colorYellow)
	return strings.Join([]string{
		orange.Render("╭─╮ ╭─╮"),
		orange.Render("│") + yellow.Render("●") + orange.Render("╰─╯") + yellow.Render("●") + orange.Render("│"),
		orange.Render("  ╰╥╯"),
		orange.Render("▟█╰") + yellow.Render("▪") + orange.Render("╯█▙"),
	}, "\n")
}

// snapshot 汇总输入框附近状态区需要的数据。
// 调用层级：View -> renderMainPane -> snapshot。
func (m *AppModel) snapshot() statusSnapshot {
	snapshot := statusSnapshot{Runtime: runtimeMeta{
		Busy:           m.busy,
		State:          animatedStateLabel(m.busy, m.currentStatus, m.spinnerFrame),
		Turn:           m.remoteTurn,
		ScrollPercent:  int(m.viewport.ScrollPercent() * 100),
		ToolCallsTotal: m.toolCalls,
		LastToolName:   m.lastTool,
		PendingInput:   strings.TrimSpace(m.pendingInput) != "",
		SessionID:      m.sessionID,
	}}
	snapshot.Runtime.Workdir, snapshot.Runtime.Git = m.runtimeLocation()
	return snapshot
}

func (m *AppModel) runtimeLocation() (string, gitMeta) {
	if m.metaCache.Workdir != "" {
		return m.metaCache.Workdir, m.metaCache.Git
	}
	return "-", gitMeta{}
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

func (m *AppModel) renderSlashHint(width int) string {
	if m.picker != nil {
		if len(m.picker.Options) == 0 {
			return slashHintStyle.Width(width).Render("No options available")
		}
		lines := []string{"Select " + m.picker.Kind}
		start := max(0, m.picker.Cursor-maxHintRows/2)
		end := min(len(m.picker.Options), start+maxHintRows)
		start = max(0, end-maxHintRows)
		for index := start; index < end; index++ {
			option := m.picker.Options[index]
			prefix := "  "
			if index == m.picker.Cursor {
				prefix = "> "
			}
			lines = append(lines, prefix+option)
		}
		if len(m.picker.Options) > end {
			lines = append(lines, fmt.Sprintf("  ... %d more", len(m.picker.Options)-end))
		}
		return slashHintStyle.Width(width).Render(strings.Join(lines, "\n"))
	}
	raw := m.input.Value()
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	if isModelHintInput(raw, text) {
		matches := m.modelHintMatches(modelArgPrefix(raw))
		if len(matches) == 0 {
			return ""
		}
		limit := min(len(matches), maxHintRows)
		lines := make([]string, 0, limit+1)
		for _, option := range matches[:limit] {
			lines = append(lines, wrapVisibleText("  "+option, max(8, width-2)))
		}
		if len(matches) > limit {
			lines = append(lines, fmt.Sprintf("  ... %d more", len(matches)-limit))
		}
		return slashHintStyle.Width(width).Render(strings.Join(lines, "\n"))
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

func isModelHintInput(raw string, trimmed string) bool {
	return trimmed == "/model" || strings.HasPrefix(trimmed, "/model ")
}

func modelArgPrefix(input string) string {
	fields := strings.Fields(input)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

func (m *AppModel) modelHintMatches(prefix string) []string {
	seen := map[string]bool{}
	matches := make([]string, 0, len(modelHints)+1)
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || !strings.Contains(ref, "/") || seen[ref] {
			return
		}
		if prefix != "" && !strings.HasPrefix(ref, prefix) {
			return
		}
		seen[ref] = true
		matches = append(matches, ref)
	}
	add(m.modelName)
	for _, hint := range modelHints {
		add(hint)
	}
	return matches
}

func (m *AppModel) renderConversationEntryCached(index int, width int) string {
	entry := m.entries[index]
	frame := -1
	if entry.Role == roleHint && entry.ToolState == "running" {
		frame = m.spinnerFrame
	}
	key := entry.renderKey()
	if entry.renderCacheText != "" && entry.renderCacheKey == key && entry.renderCacheWidth == width && entry.renderCacheFrame == frame {
		return entry.renderCacheText
	}
	rendered := m.renderConversationEntry(entry, width)
	m.entries[index].renderCacheKey = key
	m.entries[index].renderCacheWidth = width
	m.entries[index].renderCacheFrame = frame
	m.entries[index].renderCacheText = rendered
	return rendered
}

func (entry conversationEntry) renderKey() string {
	return strings.Join([]string{entry.Role, entry.Content, entry.CreatedAt, entry.ToolName, entry.ToolIntent, entry.ToolArgs, entry.ToolKey, entry.ToolState, entry.ToolOutput, entry.SystemTitle}, "\x00")
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
	return tea.Tick(180*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}
