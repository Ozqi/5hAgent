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
