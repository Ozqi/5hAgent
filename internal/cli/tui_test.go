package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lzq/5hAgent/internal/logger"
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
		func(ctx context.Context, sink func(event logger.ToolEvent)) (string, error) {
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

func TestReservedMainHeightTracksSlashHintWrapping(t *testing.T) {
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
	model.width = 50
	model.height = 18

	base := model.reservedMainHeight(50)
	model.input.SetValue("/")
	withSlash := model.reservedMainHeight(50)
	if withSlash <= base {
		t.Fatalf("reserved height with slash = %d, want > base %d", withSlash, base)
	}
}

func TestRenderMainPaneKeepsFooterWithoutSlashHint(t *testing.T) {
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
	model.width = 80
	model.height = 24
	model.resize()
	model.refreshView()
	model.metaCache = cachedMeta{Workdir: "/tmp/test-workspace", LoadedAt: time.Now()}

	rendered := stripANSI(renderMainPane(model))
	if !strings.Contains(rendered, "test-workspace") {
		t.Fatalf("rendered main pane missing footer: %q", rendered)
	}
	if strings.Contains(rendered, "session") || strings.Contains(rendered, "dir ") {
		t.Fatalf("rendered main pane contains removed footer labels: %q", rendered)
	}
	if strings.Contains(rendered, "unknown slash command") {
		t.Fatalf("rendered main pane contains slash hint without slash input: %q", rendered)
	}
}

func TestRenderFixedLinesClipsWithoutEllipsis(t *testing.T) {
	line := logger.Gray(strings.Repeat("▄", 40))
	rendered := stripANSI(renderFixedLines([]string{line}, 20, 1))

	if strings.Contains(rendered, "...") {
		t.Fatalf("rendered line = %q, should not insert ellipsis", rendered)
	}
	if got := len([]rune(strings.TrimRight(rendered, " "))); got != 20 {
		t.Fatalf("visible runes = %d, want 20 in %q", got, rendered)
	}
}

func TestToolHintEntryShowsToolNameWithoutRanVerb(t *testing.T) {
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

	rendered := stripANSI(model.renderToolHintEntry(conversationEntry{
		Role:       roleHint,
		ToolName:   "base.read_file",
		ToolArgs:   "[path=.5hagent/task.md,limit=800]",
		ToolState:  "done",
		ToolOutput: "total lines: 15",
	}, 80))

	if !strings.Contains(rendered, "base.read_file") {
		t.Fatalf("rendered tool entry = %q, want tool name", rendered)
	}
	if strings.Contains(rendered, "Ran") || strings.Contains(rendered, "Running") || strings.Contains(rendered, "Failed") {
		t.Fatalf("rendered tool entry = %q, should not contain status verb", rendered)
	}
}

func TestFormatToolArgsSummaryKeepsLongerValues(t *testing.T) {
	longPath := "/Users/bytedance/Proj/5hAgent/internal/cli/tui.go"
	summary := formatToolArgsSummary(`{"path":"` + longPath + `","limit":800}`)

	if !strings.Contains(summary, "internal/cli/tui.go") {
		t.Fatalf("summary = %q, want useful path suffix", summary)
	}
	if strings.Contains(summary, "args=") {
		t.Fatalf("summary = %q, want parsed key/value format", summary)
	}
}
