# Task Md Single Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make repository-root `task.md` the only persisted task source while keeping `/task` and `TaskTool` on the same backend.

**Architecture:** Keep `internal/agent.TaskList` as the shared service boundary, but switch persistence from JSON to a managed markdown section inside `task.md`. Reload from disk before each public read/write so manual edits are visible without restarting the app.

**Tech Stack:** Go, markdown text parsing, existing Cobra command and tool wiring

---

### Task 1: Lock markdown persistence behavior with tests

**Files:**
- Create: `internal/agent/tasklist_test.go`
- Modify: `internal/agent/tasklist.go`
- Test: `internal/agent/tasklist_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
    if !strings.Contains(text, "existing content") || !strings.Contains(text, "<!-- 5hagent:tasks:start -->") {
        t.Fatalf("unexpected markdown: %s", text)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -run TestTaskListPersistsTasksInManagedTaskMarkdownSection -v`
Expected: FAIL because `tasklist.go` still writes JSON instead of a markdown managed section

- [ ] **Step 3: Write minimal implementation**

```go
const (
    taskSectionStart = "<!-- 5hagent:tasks:start -->"
    taskSectionEnd   = "<!-- 5hagent:tasks:end -->"
)

func (tl *TaskList) save() error {
    content, err := tl.renderDocument()
    if err != nil {
        return err
    }
    return os.WriteFile(tl.filePath, []byte(content), 0o644)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -run TestTaskListPersistsTasksInManagedTaskMarkdownSection -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/tasklist.go internal/agent/tasklist_test.go doc/superpowers/plans/2026-04-23-task-md-single-source.md
git commit -m "refactor: persist task list in task.md"
```

### Task 2: Reload markdown before operations so manual edits become visible

**Files:**
- Modify: `internal/agent/tasklist.go`
- Test: `internal/agent/tasklist_test.go`

- [ ] **Step 1: Write the failing test**

```go
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

    err = os.WriteFile(path, []byte(strings.ReplaceAll(string(mustReadFile(t, path)), "pending", "blocked")), 0o644)
    if err != nil {
        t.Fatal(err)
    }

    task, err := list.GetTask("T-1")
    if err != nil {
        t.Fatal(err)
    }
    if task.Status != StatusBlocked {
        t.Fatalf("got %s", task.Status)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -run TestTaskListReloadsManualMarkdownEdits -v`
Expected: FAIL because in-memory state is stale and does not reload `task.md`

- [ ] **Step 3: Write minimal implementation**

```go
func (tl *TaskList) reloadLocked() error {
    tl.tasks = make(map[string]*Task)
    if _, err := os.Stat(tl.filePath); err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }
    return tl.load()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -run TestTaskListReloadsManualMarkdownEdits -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/tasklist.go internal/agent/tasklist_test.go
git commit -m "refactor: reload task state from task.md"
```

### Task 3: Wire the application to repository-root task.md

**Files:**
- Modify: `cmd/5hagent/main.go`
- Modify: `internal/commands/task.go`
- Modify: `internal/tools/task_tool.go`
- Test: `internal/agent/tasklist_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestTaskListSupportsNewMarkdownStatuses(t *testing.T) {
    path := filepath.Join(t.TempDir(), "task.md")
    err := os.WriteFile(path, []byte(`# Tasks

<!-- 5hagent:tasks:start -->
## Shared Tasks

### T-1 | Archived task
- status: archived
- description: archived item
- created_at: 2026-04-23T00:00:00Z
- updated_at: 2026-04-23T00:00:00Z

<!-- 5hagent:tasks:end -->
`), 0o644)
    if err != nil {
        t.Fatal(err)
    }

    list, err := NewTaskList(path)
    if err != nil {
        t.Fatal(err)
    }

    tasks := list.ListTasksByStatus(StatusArchived)
    if len(tasks) != 1 {
        t.Fatalf("got %d tasks", len(tasks))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -run TestTaskListSupportsNewMarkdownStatuses -v`
Expected: FAIL because markdown parsing and new statuses are incomplete

- [ ] **Step 3: Write minimal implementation**

```go
const (
    StatusBlocked  TaskStatus = "blocked"
    StatusArchived TaskStatus = "archived"
)

taskListPath := "task.md"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -run TestTaskListSupportsNewMarkdownStatuses -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/5hagent/main.go internal/commands/task.go internal/tools/task_tool.go internal/agent/tasklist.go internal/agent/tasklist_test.go
git commit -m "refactor: use task.md as shared task source"
```
