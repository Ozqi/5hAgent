package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lzq/5hAgent/internal/logger"
)

func newTestAppModel() *AppModel {
	return NewAppModel(context.Background(), nil, "test-model", "", nil, nil, nil, nil, "")
}

func TestToolEventsRenderAsOrderedHintsBetweenAssistantText(t *testing.T) {
	m := newTestAppModel()

	updated, _ := m.Update(assistantTokenMsg{token: "before"})
	m = updated.(*AppModel)
	updated, _ = m.Update(toolEventMsg{event: logger.ToolEvent{Kind: "call", Name: "base.read_file", Text: "\n● read_file\n  path: file.txt\n"}})
	m = updated.(*AppModel)
	updated, _ = m.Update(toolEventMsg{event: logger.ToolEvent{Kind: "result", Name: "base.read_file", Text: "  ⎿ ok\n"}})
	m = updated.(*AppModel)
	updated, _ = m.Update(assistantTokenMsg{token: "after"})
	m = updated.(*AppModel)

	if len(m.entries) != 3 {
		t.Fatalf("entries = %d, want assistant, hint, assistant", len(m.entries))
	}
	wantRoles := []string{roleAssistant, roleHint, roleAssistant}
	for i, want := range wantRoles {
		if m.entries[i].Role != want {
			t.Fatalf("entry[%d].Role = %q, want %q", i, m.entries[i].Role, want)
		}
	}
	if m.entries[0].Content != "before" || m.entries[2].Content != "after" {
		t.Fatalf("assistant contents = %q / %q, want before / after", m.entries[0].Content, m.entries[2].Content)
	}
	if strings.Contains(m.entries[0].Content, "read_file") || strings.Contains(m.entries[2].Content, "read_file") {
		t.Fatalf("tool text leaked into assistant content: %#v", m.entries)
	}
	if m.entries[1].ToolName != "read_file" || m.entries[1].ToolState != "done" {
		t.Fatalf("hint = %#v, want read_file done", m.entries[1])
	}
}

func TestToolEventWithoutAssistantCreatesHintEntry(t *testing.T) {
	m := newTestAppModel()

	updated, _ := m.Update(toolEventMsg{event: logger.ToolEvent{Kind: "call", Name: "base.glob", Text: "\n● glob\n  pattern: *.go\n"}})
	m = updated.(*AppModel)

	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.entries))
	}
	if m.entries[0].Role != roleHint {
		t.Fatalf("entry role = %q, want hint", m.entries[0].Role)
	}
	if m.entries[0].ToolName != "glob" || m.entries[0].ToolState != "running" {
		t.Fatalf("entry = %#v, want running glob tool summary", m.entries[0])
	}
}

func TestThinkingTokensRenderAsSeparateDimEntry(t *testing.T) {
	m := newTestAppModel()

	updated, _ := m.Update(assistantThinkingMsg{token: "checking context"})
	m = updated.(*AppModel)
	updated, _ = m.Update(assistantTokenMsg{token: "final answer"})
	m = updated.(*AppModel)

	if len(m.entries) != 2 {
		t.Fatalf("entries = %d, want thinking and assistant", len(m.entries))
	}
	if m.entries[0].Role != roleThinking {
		t.Fatalf("entry[0].Role = %q, want %q", m.entries[0].Role, roleThinking)
	}
	if m.entries[1].Role != roleAssistant {
		t.Fatalf("entry[1].Role = %q, want %q", m.entries[1].Role, roleAssistant)
	}

	rendered := m.renderConversationEntry(m.entries[0], 80)
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "thinking") || !strings.Contains(plain, "checking context") {
		t.Fatalf("thinking render = %q, want label and content", plain)
	}
}

func TestRenderConversationEntryUsesPlainTimelineStyle(t *testing.T) {
	m := newTestAppModel()
	user := stripANSI(m.renderConversationEntry(conversationEntry{Role: roleUser, Content: "run tools"}, 80))
	assistant := stripANSI(m.renderConversationEntry(conversationEntry{Role: roleAssistant, Content: "plain answer"}, 80))
	hintRendered := m.renderConversationEntry(conversationEntry{Role: roleHint, ToolName: "read_file", ToolArgs: "[path=file.txt]", ToolState: "done", ToolOutput: "ok"}, 80)
	hint := stripANSI(hintRendered)

	for _, forbidden := range []string{"USER_ROOT", "AGENT_CORE", "TOOL_EXEC", "assistant:", "user:"} {
		if strings.Contains(user+assistant+hint, forbidden) {
			t.Fatalf("rendered timeline contains forbidden label %q:\n%s\n%s\n%s", forbidden, user, assistant, hint)
		}
	}
	if !strings.Contains(user, "> run tools") {
		t.Fatalf("user render = %q, want shell prompt", user)
	}
	if strings.TrimSpace(assistant) != "plain answer" {
		t.Fatalf("assistant render = %q, want plain text", assistant)
	}
	if !strings.Contains(hint, "read_file [path=file.txt]") || !strings.Contains(hint, "ok") {
		t.Fatalf("hint render = %q, want tool hint", hint)
	}
}

func TestSlashHintMatchesPrefix(t *testing.T) {
	all := slashHintMatches("/")
	if len(all) < 4 {
		t.Fatalf("slashHintMatches('/') = %d, want common slash commands", len(all))
	}
	task := slashHintMatches("/ta")
	if len(task) != 1 || task[0].Name != "/task" {
		t.Fatalf("slashHintMatches('/ta') = %#v, want /task", task)
	}
	if got := slashHintMatches("/unknown"); len(got) != 0 {
		t.Fatalf("slashHintMatches('/unknown') = %#v, want none", got)
	}
}

func TestRenderSlashHintShowsUsage(t *testing.T) {
	m := newTestAppModel()
	m.input.SetValue("/m")

	rendered := stripANSI(m.renderSlashHint(80))
	if !strings.Contains(rendered, "/mcp") {
		t.Fatalf("renderSlashHint = %q, want /mcp usage", rendered)
	}
}

func TestWrapVisibleLinesPreservesANSIAndWideWidth(t *testing.T) {
	colored := logger.Green("这是一段很长的中文文本")
	wrapped := wrapVisibleLines(colored, 6)
	if len(wrapped) < 2 {
		t.Fatalf("wrapped lines = %#v, want multiple lines", wrapped)
	}
	for _, line := range wrapped {
		if strings.Contains(line, "\x1b[") && !strings.HasSuffix(line, "\x1b[0m") {
			t.Fatalf("wrapped line has unterminated ANSI sequence: %q", line)
		}
		if width := lipgloss.Width(line); width > 6 {
			t.Fatalf("wrapped line %q width = %d, want <= 6", line, width)
		}
	}
}
