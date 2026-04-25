package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteTaskActionRejectsInvalidStatus(t *testing.T) {
	list, err := NewTaskList(filepath.Join(t.TempDir(), "task.md"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "create", ID: "T-1", Title: "task", Description: "desc"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "update", ID: "T-1", Status: "wat"})
	if err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestExecuteTaskActionListRejectsInvalidFilter(t *testing.T) {
	list, err := NewTaskList(filepath.Join(t.TempDir(), "task.md"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "list", Status: "wat"})
	if err == nil {
		t.Fatal("expected invalid status filter error")
	}
}

func TestExecuteTaskActionReturnsStructuredListProgress(t *testing.T) {
	list, err := NewTaskList(filepath.Join(t.TempDir(), "task.md"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "create", ID: "T-1", Title: "task", Description: "desc"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteTaskAction(list, TaskActionRequest{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path == "" {
		t.Fatal("expected task source path in list result")
	}
	if result.Progress.Total != 1 || result.Progress.Pending != 1 {
		t.Fatalf("unexpected progress: %+v", result.Progress)
	}
	if len(result.Tasks) != 1 || result.Tasks[0].ID != "T-1" {
		t.Fatalf("unexpected tasks: %+v", result.Tasks)
	}
	if result.Source != "task.md" {
		t.Fatalf("expected source task.md, got %q", result.Source)
	}
	if result.Task == nil {
		t.Fatal("expected primary task in list result")
	}
}

func TestExecuteTaskActionArchiveWritesHistoryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")
	list, err := NewTaskList(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "create", ID: "T-1", Title: "task", Description: "finish archive flow"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "update", ID: "T-1", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteTaskAction(list, TaskActionRequest{Action: "archive", ID: "T-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil {
		t.Fatal("expected archived task in result")
	}
	if result.Task.Status != StatusArchived {
		t.Fatalf("expected archived status, got %s", result.Task.Status)
	}
	if result.Task.HistoryPath == "" {
		t.Fatal("expected history path on archived task")
	}
	if result.Task.Summary == "" {
		t.Fatal("expected summary on archived task")
	}

	historyFile := filepath.Join(filepath.Dir(path), result.Task.HistoryPath)
	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# History Task T-1") {
		t.Fatalf("expected history title, got %q", text)
	}
	if !strings.Contains(text, "## Original Goal") {
		t.Fatalf("expected original goal section, got %q", text)
	}
	if !strings.Contains(text, "finish archive flow") {
		t.Fatalf("expected original description in history file, got %q", text)
	}

	taskMarkdown, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	taskText := string(taskMarkdown)
	if !strings.Contains(taskText, "- status: archived") {
		t.Fatalf("expected archived entry in task.md, got %q", taskText)
	}
	if !strings.Contains(taskText, "- summary: ") {
		t.Fatalf("expected summary in task.md, got %q", taskText)
	}
	if !strings.Contains(taskText, "- history: ") {
		t.Fatalf("expected history link in task.md, got %q", taskText)
	}
	if strings.Contains(taskText, "- description: finish archive flow") {
		t.Fatalf("expected archived entry to drop description, got %q", taskText)
	}
}

func TestExecuteTaskActionReopenRestoresArchivedTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")
	list, err := NewTaskList(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "create", ID: "T-1", Title: "task", Description: "finish archive flow"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "update", ID: "T-1", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := ExecuteTaskAction(list, TaskActionRequest{Action: "archive", ID: "T-1"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteTaskAction(list, TaskActionRequest{Action: "reopen", ID: "T-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil {
		t.Fatal("expected reopened task in result")
	}
	if result.Task.Status != StatusPending {
		t.Fatalf("expected pending status, got %s", result.Task.Status)
	}
	if result.Task.RestoredFrom != archived.Task.HistoryPath {
		t.Fatalf("expected restored_from %q, got %q", archived.Task.HistoryPath, result.Task.RestoredFrom)
	}
	if result.Task.Description != "finish archive flow" {
		t.Fatalf("expected restored description, got %q", result.Task.Description)
	}
	if result.Task.HistoryPath != "" {
		t.Fatalf("expected history path cleared after reopen, got %q", result.Task.HistoryPath)
	}
	if result.Task.Summary != "" {
		t.Fatalf("expected summary cleared after reopen, got %q", result.Task.Summary)
	}
}

func TestExecuteTaskActionReopenRejectsArchivedStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")
	list, err := NewTaskList(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "create", ID: "T-1", Title: "task", Description: "finish archive flow"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "update", ID: "T-1", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "archive", ID: "T-1"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ExecuteTaskAction(list, TaskActionRequest{Action: "reopen", ID: "T-1", Status: "archived"})
	if err == nil {
		t.Fatal("expected reopen with archived status to fail")
	}
}
