package tui

import (
	"context"
	"github.com/lzq/5hAgent/internal/toolevent"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRunCommandStartsTaskRunner(t *testing.T) {
	called := false
	model := NewAppModel(
		context.Background(),
		nil,
		"test-model",
		"",
		nil,
		nil,
		nil,
		nil,
		"test-session",
		func(ctx context.Context, sink func(event toolevent.ToolEvent)) (string, error) {
			called = true
			return "Run completed: 1 task(s)", nil
		},
		nil,
	)
	cmd := model.handleRunCommand("/run")
	if cmd == nil {
		t.Fatal("handleRunCommand() returned nil command")
	}
	if !model.busy || model.currentStatus != "running tasks" {
		t.Fatalf("state = busy:%v status:%s, want running tasks", model.busy, model.currentStatus)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("message = %T, want two-command BatchMsg", msg)
	}
	done, ok := batch[1]().(runTasksDoneMsg)
	if !ok {
		t.Fatalf("message = %T, want runTasksDoneMsg", msg)
	}
	if !called {
		t.Fatal("runTasks was not called")
	}
	if !strings.Contains(done.summary, "Run completed") {
		t.Fatalf("summary = %q, want run summary", done.summary)
	}
}

func TestModelCommandSwitchesModel(t *testing.T) {
	var gotRef string
	model := NewAppModel(
		context.Background(),
		nil,
		"old-model",
		"",
		nil,
		nil,
		nil,
		nil,
		"test-session",
		nil,
		func(ctx context.Context, ref string) (string, error) {
			gotRef = ref
			return "gpt-5.5", nil
		},
	)

	cmd := model.handleModelCommand("/model mira/gpt-5.5")
	if cmd != nil {
		t.Fatalf("handleModelCommand() cmd = %v, want nil", cmd)
	}
	if gotRef != "mira/gpt-5.5" {
		t.Fatalf("model ref = %q, want mira/gpt-5.5", gotRef)
	}
	if model.modelName != "gpt-5.5" {
		t.Fatalf("modelName = %q, want gpt-5.5", model.modelName)
	}
	if len(model.entries) == 0 || !strings.Contains(model.entries[len(model.entries)-1].Content, "Switched model") {
		t.Fatalf("entries = %+v, want switched message", model.entries)
	}
}

func TestModelCommandShowsUsage(t *testing.T) {
	model := NewAppModel(
		context.Background(),
		nil,
		"old-model",
		"",
		nil,
		nil,
		nil,
		nil,
		"test-session",
		nil,
		nil,
	)

	model.handleModelCommand("/model")
	if len(model.entries) == 0 {
		t.Fatal("entries empty, want usage")
	}
	got := model.entries[len(model.entries)-1].Content
	if !strings.Contains(got, "usage: /model <provider/model>") || !strings.Contains(got, "mira/gpt-5.4") {
		t.Fatalf("usage = %q, want model examples", got)
	}
}

func TestStopCommandCancelsActiveRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := NewAppModel(context.Background(), nil, "test-model", "", nil, nil, nil, nil, "test-session", nil, nil)
	model.busy = true
	model.runCancel = cancel
	model.input.SetValue("/stop")

	if cmd := model.submit(); cmd != nil {
		t.Fatalf("submit(/stop) cmd = %v, want nil", cmd)
	}
	if ctx.Err() == nil {
		t.Fatal("run context was not canceled")
	}
	if model.busy {
		t.Fatal("busy = true, want false after stop")
	}
	if model.currentStatus != "stopped" {
		t.Fatalf("currentStatus = %q, want stopped", model.currentStatus)
	}
	if len(model.entries) == 0 || !strings.Contains(model.entries[len(model.entries)-1].Content, "stopped current run") {
		t.Fatalf("entries = %#v, want stop message", model.entries)
	}
}

func TestStopCommandReportsNoActiveRun(t *testing.T) {
	model := NewAppModel(context.Background(), nil, "test-model", "", nil, nil, nil, nil, "test-session", nil, nil)
	model.input.SetValue("/stop")

	if cmd := model.submit(); cmd != nil {
		t.Fatalf("submit(/stop) cmd = %v, want nil", cmd)
	}
	if len(model.entries) == 0 || !strings.Contains(model.entries[len(model.entries)-1].Content, "no active run") {
		t.Fatalf("entries = %#v, want no active run message", model.entries)
	}
}

func TestAssistantTokenIgnoredAfterStop(t *testing.T) {
	model := NewAppModel(context.Background(), nil, "test-model", "", nil, nil, nil, nil, "test-session", nil, nil)
	model.busy = false

	updated, _ := model.Update(assistantTokenMsg{token: "late"})
	model = updated.(*AppModel)
	if len(model.entries) != 0 {
		t.Fatalf("entries = %#v, want no late assistant token", model.entries)
	}
}

func TestRefreshViewSeparatesConversationEntries(t *testing.T) {
	model := NewAppModel(context.Background(), nil, "test-model", "", nil, nil, nil, nil, "test-session", nil, nil)
	model.width = 80
	model.height = 24
	model.entries = []conversationEntry{
		{Role: roleUser, Content: "first"},
		{Role: roleAssistant, Content: "second"},
	}

	model.refreshView()

	if !strings.Contains(stripANSI(model.viewText), "\n\n") {
		t.Fatalf("viewText = %q, want a blank line between conversation entries", model.viewText)
	}
}

func TestRenderTokenStatusShowsContextAndSessionUsage(t *testing.T) {
	got := renderTokenStatus(runtimeMeta{ContextTokens: 1200, ContextWindow: 32768, SessionTokens: 4500})
	if got != "ctx 1200/32768 tokens · total 4500 tokens · spent $--" {
		t.Fatalf("renderTokenStatus() = %q", got)
	}
}

func TestConfirmRequiresSecondPressWithinWindow(t *testing.T) {
	pending := false
	var last time.Time

	if confirm(&pending, &last, time.Second) {
		t.Fatal("first confirm() = true, want false")
	}
	if !pending || last.IsZero() {
		t.Fatalf("pending=%v last=%v, want pending state", pending, last)
	}
	if !confirm(&pending, &last, time.Second) {
		t.Fatal("second confirm() = false, want true")
	}
	if pending {
		t.Fatal("pending = true after confirmed, want false")
	}
}

func TestQuitConfirmDelayAllowsHumanSecondPress(t *testing.T) {
	pending := true
	last := time.Now().Add(-time.Second)

	if !confirm(&pending, &last, quitConfirmDelay) {
		t.Fatal("confirm() = false after 1s, want true")
	}
}

func TestQuitConfirmSurvivesNonKeyMessage(t *testing.T) {
	model := NewAppModel(
		context.Background(),
		nil,
		"test-model",
		"",
		nil,
		nil,
		nil,
		nil,
		"test-session",
		nil,
		nil,
	)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatalf("first ctrl+c cmd = %v, want nil", cmd)
	}
	model = updated.(*AppModel)
	model.Update(spinnerTickMsg{})
	if !model.quitPending {
		t.Fatal("quitPending = false after non-key message, want true")
	}
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("second ctrl+c cmd = nil, want tea.Quit")
	}
}
