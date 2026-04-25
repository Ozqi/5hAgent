package cli

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lzq/5hAgent/internal/skill"
)

func TestRenderStatusPanelIncludesKeySections(t *testing.T) {
	panel := renderStatusPanel(statusSnapshot{
		Busy:                true,
		CurrentState:        "streaming",
		TokenUsed:           128,
		TokenLimit:          1000,
		ContextMessages:     12,
		ContextSummaries:    1,
		ToolCallsTotal:      7,
		LastToolName:        "base.read_file",
		EnabledSkills:       []string{"using-superpowers", "brainstorming"},
		TaskTotal:           3,
		TaskInProgress:      1,
		HighlightedTaskLine: []string{"task-1 in_progress", "task-2 pending"},
	}, 28)

	checks := []string{
		"status",
		"busy",
		"state: streaming",
		"context",
		"tokens",
		"128/1000",
		"messages: 12",
		"summaries: 1",
		"tools",
		"calls: 7",
		"last: read_file",
		"skills",
		"using-superpowers",
		"tasks",
		"total: 3",
		"in progress: 1",
		"task-1 in_progress",
	}

	for _, check := range checks {
		if !strings.Contains(panel, check) {
			t.Fatalf("expected panel to contain %q, got %q", check, panel)
		}
	}
}

func TestRenderConversationEntryUsesMarkdownRenderer(t *testing.T) {
	got := renderConversationEntry(conversationEntry{
		Role:    roleAssistant,
		Content: "# Title\n\nHello\nworld with `code`",
	})

	if !strings.Contains(got, "Agent") {
		t.Fatalf("expected agent prefix, got %q", got)
	}
	if !strings.Contains(got, "Title") || !strings.Contains(got, "Hello world") {
		t.Fatalf("expected markdown content rendered, got %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Fatalf("expected assistant markdown to collapse blank lines, got %q", got)
	}
	if !strings.Contains(got, "\033[") {
		t.Fatalf("expected ansi styling in rendered entry, got %q", got)
	}
}

func TestRenderToolEntryIsDimAndCompact(t *testing.T) {
	got := renderConversationEntry(conversationEntry{
		Role:    roleTool,
		Content: "\n● base.read_file\n  path: foo/bar.txt\n  lines: 1-20\n",
	})

	if !strings.Contains(got, "tool") || !strings.Contains(got, "read_file") {
		t.Fatalf("expected compact tool entry, got %q", got)
	}
	if strings.Contains(got, "●") {
		t.Fatalf("expected raw tool bullet to be normalized, got %q", got)
	}
	if !strings.Contains(got, "\033[") {
		t.Fatalf("expected styled tool entry, got %q", got)
	}
}

func TestAnimatedStateLabel(t *testing.T) {
	got := animatedStateLabel(true, "thinking", 1)
	if !strings.Contains(got, "thinking") || !strings.Contains(got, spinnerFrames[1]) {
		t.Fatalf("expected animated state label, got %q", got)
	}
}

func TestInitOnlyStartsCursorBlink(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)

	if cmd := m.Init(); cmd == nil {
		t.Fatal("expected init to return cursor blink command")
	}

	if m.spinnerFrame != 0 {
		t.Fatalf("expected init not to advance spinner frame, got %d", m.spinnerFrame)
	}
}

func TestDoubleEscQuits(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	m.width = 100
	m.height = 30

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("expected first esc not to quit")
	}
	if !m.escPending {
		t.Fatal("expected first esc to arm quit state")
	}

	m.lastEscAt = time.Now()
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected second esc to return quit command")
	}
}

func TestEnterSubmitsMessage(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	m.width = 100
	m.height = 30
	m.input.SetValue("hello")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.entries) != 1 {
		t.Fatalf("expected only user entry after submit, got %d", len(m.entries))
	}
	if m.entries[0].Role != roleUser || m.entries[0].Content != "hello" {
		t.Fatalf("unexpected first entry: %+v", m.entries[0])
	}
	if m.currentAssistant != -1 {
		t.Fatalf("expected no assistant placeholder before first token, got %d", m.currentAssistant)
	}
	if view := m.View(); !strings.Contains(view, "You:") || !strings.Contains(view, "hello") {
		t.Fatalf("expected submitted user text to render immediately, got %q", view)
	}
}

func TestTypingUpdatesInputValueAndView(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	m.width = 100
	m.height = 30
	m.resize()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if got := m.input.Value(); got != "hi" {
		t.Fatalf("expected input value hi, got %q", got)
	}
	view := m.View()
	if !strings.Contains(view, "hi") {
		t.Fatalf("expected input text to appear in view, got %q", view)
	}
}

func TestAssistantEntryCreatedOnFirstToken(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	m.width = 100
	m.height = 30
	m.input.SetValue("hello")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, _ = m.Update(assistantTokenMsg{token: "world"})

	if len(m.entries) != 2 {
		t.Fatalf("expected user and assistant entries after first token, got %d", len(m.entries))
	}
	if m.entries[1].Role != roleAssistant || m.entries[1].Content != "world" {
		t.Fatalf("unexpected assistant entry: %+v", m.entries[1])
	}
}

func TestRenderConversationEntryTrimsTrailingAssistantNewlines(t *testing.T) {
	got := renderConversationEntry(conversationEntry{
		Role:    roleAssistant,
		Content: "Hello\n\n",
	})

	if strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected assistant entry not to end with extra blank lines, got %q", got)
	}
}

func TestSubmitStartsSpinnerTick(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	m.width = 100
	m.height = 30
	m.input.SetValue("hello")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit to start spinner tick command")
	}
	if !m.busy {
		t.Fatal("expected submit to mark model busy")
	}
	if got := animatedStateLabel(m.busy, m.currentStatus, 0); !strings.Contains(got, spinnerFrames[0]) {
		t.Fatalf("expected animated state label after submit, got %q", got)
	}
}

func TestInitialSubmitRendersConversationBeforeWindowSize(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	m.input.SetValue("hello")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m.View()
	if !strings.Contains(view, "You:") || !strings.Contains(view, "hello") {
		t.Fatalf("expected initial submit to render conversation before window size, got %q", view)
	}
}

func TestInitialSubmitShowsSpinnerBeforeWindowSize(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	m.input.SetValue("hello")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m.View()
	if !strings.Contains(view, "thinking") || !strings.Contains(view, spinnerFrames[0]) {
		t.Fatalf("expected initial submit to show spinner before window size, got %q", view)
	}
}

func TestViewDoesNotExceedWindowHeight(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()
	lines := strings.Count(view, "\n") + 1
	if lines > 30 {
		t.Fatalf("expected rendered view to fit window height, got %d lines for height 30\n%s", lines, view)
	}
}

func TestSubmittedMessageRemainsVisibleAfterWindowSize(t *testing.T) {
	m := NewAppModel(nil, nil, "", nil, skill.NewManager(""), nil, nil)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	m.input.SetValue("hello")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m.View()
	if !strings.Contains(view, "You:") || !strings.Contains(view, "hello") {
		t.Fatalf("expected submitted message to remain visible after layout, got %q", view)
	}
}

func TestNormalizeAssistantContentCollapsesBlankLines(t *testing.T) {
	got := normalizeAssistantContent("a\n\nb\n\n\nc")
	if got != "a\nb\nc" {
		t.Fatalf("expected collapsed blank lines, got %q", got)
	}
}
