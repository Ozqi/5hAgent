package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/lzq/5hAgent/internal/logger"
)

func TestToolEventsRenderAsOrderedHintsBetweenAssistantText(t *testing.T) {
	m := NewAppModel(context.Background(), nil, "test-model", nil, nil, nil, nil, "")

	updated, _ := m.Update(assistantTokenMsg{token: "before"})
	m = updated.(*AppModel)
	updated, _ = m.Update(toolEventMsg{event: logger.ToolEvent{Kind: "call", Name: "base.read_file", Text: "\n● read_file\n  path: file.txt\n"}})
	m = updated.(*AppModel)
	updated, _ = m.Update(toolEventMsg{event: logger.ToolEvent{Kind: "result", Name: "base.read_file", Text: "  ⎿ ok\n"}})
	m = updated.(*AppModel)
	updated, _ = m.Update(assistantTokenMsg{token: "after"})
	m = updated.(*AppModel)

	if len(m.entries) != 4 {
		t.Fatalf("entries = %d, want assistant, hint, hint, assistant", len(m.entries))
	}
	wantRoles := []string{roleAssistant, roleHint, roleHint, roleAssistant}
	for i, want := range wantRoles {
		if m.entries[i].Role != want {
			t.Fatalf("entry[%d].Role = %q, want %q", i, m.entries[i].Role, want)
		}
	}
	if m.entries[0].Content != "before" || m.entries[3].Content != "after" {
		t.Fatalf("assistant contents = %q / %q, want before / after", m.entries[0].Content, m.entries[3].Content)
	}
	if strings.Contains(m.entries[0].Content, "read_file") || strings.Contains(m.entries[3].Content, "read_file") {
		t.Fatalf("tool text leaked into assistant content: %#v", m.entries)
	}
	if !strings.Contains(m.entries[1].Content, "read_file") || !strings.Contains(m.entries[2].Content, "done") {
		t.Fatalf("hint contents = %q / %q, want tool call and result", m.entries[1].Content, m.entries[2].Content)
	}
}

func TestToolEventWithoutAssistantCreatesHintEntry(t *testing.T) {
	m := NewAppModel(context.Background(), nil, "test-model", nil, nil, nil, nil, "")

	updated, _ := m.Update(toolEventMsg{event: logger.ToolEvent{Kind: "call", Name: "base.glob", Text: "\n● glob\n  pattern: *.go\n"}})
	m = updated.(*AppModel)

	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.entries))
	}
	if m.entries[0].Role != roleHint {
		t.Fatalf("entry role = %q, want hint", m.entries[0].Role)
	}
	if !strings.Contains(m.entries[0].Content, "glob") {
		t.Fatalf("entry content = %q, want tool summary", m.entries[0].Content)
	}
}

func TestRenderConversationEntryUsesPlainTimelineStyle(t *testing.T) {
	user := stripANSI(renderConversationEntry(conversationEntry{Role: roleUser, Content: "run tools"}, 80))
	assistant := stripANSI(renderConversationEntry(conversationEntry{Role: roleAssistant, Content: "plain answer"}, 80))
	hintRendered := renderConversationEntry(conversationEntry{Role: roleHint, Content: "[tool] read_file file.txt"}, 80)
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
	if !strings.Contains(hint, "· [tool] read_file file.txt") {
		t.Fatalf("hint render = %q, want bullet tool hint", hint)
	}
	if hintRendered == hint {
		t.Fatalf("hint render = %q, want ANSI color for [tool] and tool name", hintRendered)
	}
}
