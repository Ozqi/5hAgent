package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskListBootstrapsTaskMarkdownTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")

	_, err := NewTaskList(path)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	checks := []string{
		"# Shared Task List",
		"single source of truth",
		"<!-- 5hagent:tasks:start -->",
		"## Shared Tasks",
		"<!-- 5hagent:tasks:end -->",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("expected bootstrapped task.md to contain %q, got %q", check, text)
		}
	}
}

func TestTaskListPersistsTasksInManagedTaskMarkdownSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")
	err := os.WriteFile(path, []byte("# Notes\n\nexisting content\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	list, err := NewTaskList(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = list.CreateTask("T-1", "Root task", "Persist in markdown")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	if !strings.Contains(text, "existing content") {
		t.Fatalf("expected existing content to be preserved, got %q", text)
	}
	if !strings.Contains(text, "<!-- 5hagent:tasks:start -->") {
		t.Fatalf("expected managed task section, got %q", text)
	}
	if !strings.Contains(text, "### T-1 | Root task") {
		t.Fatalf("expected task heading in markdown, got %q", text)
	}
	if !strings.Contains(text, "- status: pending") {
		t.Fatalf("expected task status in markdown, got %q", text)
	}
}

func TestTaskListReloadsManualMarkdownEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")
	list, err := NewTaskList(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = list.CreateTask("T-1", "Root task", "First version")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	updated := strings.Replace(string(data), "- status: pending", "- status: blocked", 1)
	err = os.WriteFile(path, []byte(updated), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	task, err := list.GetTask("T-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusBlocked {
		t.Fatalf("expected blocked status after reload, got %s", task.Status)
	}
}

func TestTaskListSupportsNewMarkdownStatuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")
	content := `# Tasks

<!-- 5hagent:tasks:start -->
## Shared Tasks

### T-1 | Archived task
- status: archived
- description: archived item
- created_at: 2026-04-23T00:00:00Z
- updated_at: 2026-04-23T00:00:00Z

<!-- 5hagent:tasks:end -->
`
	err := os.WriteFile(path, []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	list, err := NewTaskList(path)
	if err != nil {
		t.Fatal(err)
	}

	tasks := list.ListTasksByStatus(StatusArchived)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 archived task, got %d", len(tasks))
	}
	if tasks[0].Status != StatusArchived {
		t.Fatalf("expected archived status, got %s", tasks[0].Status)
	}
}
