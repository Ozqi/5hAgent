package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusBlocked    TaskStatus = "blocked"
	StatusCompleted  TaskStatus = "completed"
	StatusArchived   TaskStatus = "archived"
	StatusFailed     TaskStatus = "failed"
)

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TaskList struct {
	path  string
	mu    sync.RWMutex
	tasks map[string]*Task
}

const (
	taskSectionStart = "<!-- 5hagent:tasks:start -->"
	taskSectionEnd   = "<!-- 5hagent:tasks:end -->"
)

func NewTaskList(path string) (*TaskList, error) {
	if path == "" {
		return nil, fmt.Errorf("task path is required")
	}
	list := &TaskList{path: path, tasks: map[string]*Task{}}
	if err := list.load(); err != nil {
		return nil, err
	}
	if err := list.save(); err != nil {
		return nil, err
	}
	return list, nil
}

func (l *TaskList) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *TaskList) CreateTask(id, title, desc string) (*Task, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.loadLocked(); err != nil {
		return nil, err
	}
	if _, exists := l.tasks[id]; exists {
		return nil, fmt.Errorf("task %q already exists", id)
	}
	now := time.Now().UTC()
	task := &Task{ID: id, Title: title, Description: desc, Status: StatusPending, CreatedAt: now, UpdatedAt: now}
	l.tasks[id] = task
	if err := l.saveLocked(); err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

func (l *TaskList) UpdateTaskStatus(id string, status TaskStatus) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.loadLocked(); err != nil {
		return err
	}
	task, ok := l.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	task.Status = status
	task.UpdatedAt = time.Now().UTC()
	return l.saveLocked()
}

func (l *TaskList) GetTask(id string) (*Task, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.loadLocked(); err != nil {
		return nil, err
	}
	task, ok := l.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	return cloneTask(task), nil
}

func (l *TaskList) ListTasks() []*Task {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.loadLocked()
	return l.sortedTasksLocked("")
}

func (l *TaskList) ListTasksByStatus(status TaskStatus) []*Task {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.loadLocked()
	return l.sortedTasksLocked(status)
}

func (l *TaskList) DeleteTask(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.loadLocked(); err != nil {
		return err
	}
	if _, ok := l.tasks[id]; !ok {
		return fmt.Errorf("task %q not found", id)
	}
	delete(l.tasks, id)
	return l.saveLocked()
}

func (l *TaskList) GetProgress() (total, pending, inProgress, blocked, completed, archived int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.loadLocked()
	for _, task := range l.tasks {
		total++
		switch task.Status {
		case StatusPending:
			pending++
		case StatusInProgress:
			inProgress++
		case StatusBlocked:
			blocked++
		case StatusCompleted:
			completed++
		case StatusArchived:
			archived++
		}
	}
	return total, pending, inProgress, blocked, completed, archived
}

func (l *TaskList) load() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loadLocked()
}

func (l *TaskList) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("create task dir: %w", err)
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			l.tasks = map[string]*Task{}
			return nil
		}
		return fmt.Errorf("read task file: %w", err)
	}
	l.tasks = parseTasksMarkdown(string(data))
	return nil
}

func (l *TaskList) save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.saveLocked()
}

func (l *TaskList) saveLocked() error {
	content := ""
	if data, err := os.ReadFile(l.path); err == nil {
		content = string(data)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read task file: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		content = defaultTaskMarkdownTemplate()
	}

	section := renderTasksMarkdown(l.sortedTasksLocked(""))
	updated := replaceManagedSection(content, section)
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("create task dir: %w", err)
	}
	if err := os.WriteFile(l.path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write task file: %w", err)
	}
	return nil
}

func ParseTaskStatus(raw string) (TaskStatus, error) {
	status := TaskStatus(strings.TrimSpace(raw))
	switch status {
	case StatusPending, StatusInProgress, StatusBlocked, StatusCompleted, StatusArchived:
		return status, nil
	default:
		return "", fmt.Errorf("invalid task status: %s", raw)
	}
}

func (l *TaskList) sortedTasksLocked(status TaskStatus) []*Task {
	tasks := make([]*Task, 0, len(l.tasks))
	for _, task := range l.tasks {
		if status != "" && task.Status != status {
			continue
		}
		tasks = append(tasks, cloneTask(task))
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	clone := *task
	return &clone
}

func replaceManagedSection(content, section string) string {
	trimmed := strings.TrimRight(content, "\n")
	managed := section
	start := strings.Index(trimmed, taskSectionStart)
	end := strings.Index(trimmed, taskSectionEnd)
	if start >= 0 && end >= start {
		end += len(taskSectionEnd)
		return strings.TrimRight(trimmed[:start], "\n") + "\n\n" + managed + "\n"
	}
	if trimmed == "" {
		return managed + "\n"
	}
	return trimmed + "\n\n" + managed + "\n"
}

func defaultTaskMarkdownTemplate() string {
	return "# Shared Task List\n\n> This file is the single source of truth for active 5hAgent tasks.\n> Edit task entries carefully and keep the managed markers intact.\n"
}

func renderTasksMarkdown(tasks []*Task) string {
	var b strings.Builder
	b.WriteString(taskSectionStart)
	b.WriteString("\n## Shared Tasks\n\n")
	for _, task := range tasks {
		b.WriteString(fmt.Sprintf("### %s | %s\n", task.ID, task.Title))
		b.WriteString(fmt.Sprintf("- status: %s\n", task.Status))
		b.WriteString(fmt.Sprintf("- description: %s\n", task.Description))
		b.WriteString(fmt.Sprintf("- created_at: %s\n", task.CreatedAt.UTC().Format(time.RFC3339)))
		b.WriteString(fmt.Sprintf("- updated_at: %s\n\n", task.UpdatedAt.UTC().Format(time.RFC3339)))
	}
	b.WriteString(taskSectionEnd)
	return b.String()
}

func parseTasksMarkdown(content string) map[string]*Task {
	tasks := map[string]*Task{}
	start := strings.Index(content, taskSectionStart)
	end := strings.Index(content, taskSectionEnd)
	if start < 0 || end < start {
		return tasks
	}
	section := content[start+len(taskSectionStart) : end]
	lines := strings.Split(section, "\n")
	var current *Task
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "### ") {
			if current != nil && current.ID != "" {
				tasks[current.ID] = current
			}
			body := strings.TrimPrefix(line, "### ")
			parts := strings.SplitN(body, " | ", 2)
			current = &Task{ID: strings.TrimSpace(parts[0]), Status: StatusPending}
			if len(parts) > 1 {
				current.Title = strings.TrimSpace(parts[1])
			}
			continue
		}
		if current == nil || !strings.HasPrefix(line, "- ") {
			continue
		}
		kv := strings.SplitN(strings.TrimPrefix(line, "- "), ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		switch key {
		case "status":
			current.Status = TaskStatus(value)
		case "description":
			current.Description = value
		case "created_at":
			if ts, err := time.Parse(time.RFC3339, value); err == nil {
				current.CreatedAt = ts
			}
		case "updated_at":
			if ts, err := time.Parse(time.RFC3339, value); err == nil {
				current.UpdatedAt = ts
			}
		}
	}
	if current != nil && current.ID != "" {
		tasks[current.ID] = current
	}
	return tasks
}
