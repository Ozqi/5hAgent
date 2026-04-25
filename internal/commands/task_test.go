package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lzq/5hAgent/internal/agent"
)

func TestHandleTaskArchiveAndReopen(t *testing.T) {
	list, err := agent.NewTaskList(filepath.Join(t.TempDir(), "task.md"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := agent.ExecuteTaskAction(list, agent.TaskActionRequest{Action: "create", ID: "T-1", Title: "task", Description: "finish archive flow"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.ExecuteTaskAction(list, agent.TaskActionRequest{Action: "update", ID: "T-1", Status: "completed"}); err != nil {
		t.Fatal(err)
	}

	archiveOut, err := HandleTask("/task archive T-1", list)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(archiveOut, "task \"T-1\" archived") {
		t.Fatalf("expected archive output, got %q", archiveOut)
	}

	reopenOut, err := HandleTask("/task reopen T-1", list)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reopenOut, "task \"T-1\" reopened") {
		t.Fatalf("expected reopen output, got %q", reopenOut)
	}
}
