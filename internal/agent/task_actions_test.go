package agent

import (
	"path/filepath"
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
