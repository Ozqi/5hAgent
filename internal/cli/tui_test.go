package cli

import (
	"context"
	"strings"
	"testing"

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
