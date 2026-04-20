// Package tasklist 提供任务列表管理功能
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"     // 待处理
	StatusInProgress TaskStatus = "in_progress" // 进行中
	StatusCompleted  TaskStatus = "completed"   // 已完成
	StatusFailed     TaskStatus = "failed"      // 失败
)

// Task 任务结构
type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TaskList 任务列表管理器
type TaskList struct {
	tasks    map[string]*Task
	filePath string
	mu       sync.RWMutex
}

// NewTaskList 创建新的任务列表管理器
// 参数:
//   - filePath: 任务列表持久化文件路径
//
// 返回: TaskList 实例和可能的错误
// 功能: 创建任务列表管理器，如果文件存在则加载
func NewTaskList(filePath string) (*TaskList, error) {
	tl := &TaskList{
		tasks:    make(map[string]*Task),
		filePath: filePath,
	}

	// 如果文件存在，加载任务
	if _, err := os.Stat(filePath); err == nil {
		if err := tl.load(); err != nil {
			return nil, fmt.Errorf("failed to load tasks: %w", err)
		}
	}

	return tl, nil
}

// CreateTask 创建新任务
// 参数:
//   - id: 任务 ID
//   - title: 任务标题
//   - description: 任务描述
//
// 返回: 创建的任务和可能的错误
func (tl *TaskList) CreateTask(id, title, description string) (*Task, error) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	// 检查 ID 是否已存在
	if _, exists := tl.tasks[id]; exists {
		return nil, fmt.Errorf("task with id %s already exists", id)
	}

	now := time.Now()
	task := &Task{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tl.tasks[id] = task

	// 持久化
	if err := tl.save(); err != nil {
		return nil, fmt.Errorf("failed to save tasks: %w", err)
	}

	return task, nil
}

// GetTask 获取任务
// 参数:
//   - id: 任务 ID
//
// 返回: 任务和可能的错误
func (tl *TaskList) GetTask(id string) (*Task, error) {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	task, exists := tl.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", id)
	}

	return task, nil
}

// UpdateTaskStatus 更新任务状态
// 参数:
//   - id: 任务 ID
//   - status: 新状态
//
// 返回: 可能的错误
func (tl *TaskList) UpdateTaskStatus(id string, status TaskStatus) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	task, exists := tl.tasks[id]
	if !exists {
		return fmt.Errorf("task not found: %s", id)
	}

	task.Status = status
	task.UpdatedAt = time.Now()

	// 如果标记为完成，记录完成时间
	if status == StatusCompleted {
		now := time.Now()
		task.CompletedAt = &now
	}

	// 持久化
	if err := tl.save(); err != nil {
		return fmt.Errorf("failed to save tasks: %w", err)
	}

	return nil
}

// ListTasks 列出所有任务
// 返回: 任务列表
func (tl *TaskList) ListTasks() []*Task {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	tasks := make([]*Task, 0, len(tl.tasks))
	for _, task := range tl.tasks {
		tasks = append(tasks, task)
	}

	return tasks
}

// ListTasksByStatus 按状态列出任务
// 参数:
//   - status: 任务状态
//
// 返回: 任务列表
func (tl *TaskList) ListTasksByStatus(status TaskStatus) []*Task {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	tasks := make([]*Task, 0)
	for _, task := range tl.tasks {
		if task.Status == status {
			tasks = append(tasks, task)
		}
	}

	return tasks
}

// DeleteTask 删除任务
// 参数:
//   - id: 任务 ID
//
// 返回: 可能的错误
func (tl *TaskList) DeleteTask(id string) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if _, exists := tl.tasks[id]; !exists {
		return fmt.Errorf("task not found: %s", id)
	}

	delete(tl.tasks, id)

	// 持久化
	if err := tl.save(); err != nil {
		return fmt.Errorf("failed to save tasks: %w", err)
	}

	return nil
}

// save 保存任务列表到文件
func (tl *TaskList) save() error {
	// 创建父目录
	dir := filepath.Dir(tl.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 序列化任务列表
	data, err := json.MarshalIndent(tl.tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tasks: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(tl.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// load 从文件加载任务列表
func (tl *TaskList) load() error {
	data, err := os.ReadFile(tl.filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := json.Unmarshal(data, &tl.tasks); err != nil {
		return fmt.Errorf("failed to unmarshal tasks: %w", err)
	}

	return nil
}

// GetProgress 获取任务进度统计
// 返回: 总任务数、待处理、进行中、已完成、失败
func (tl *TaskList) GetProgress() (total, pending, inProgress, completed, failed int) {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	total = len(tl.tasks)
	for _, task := range tl.tasks {
		switch task.Status {
		case StatusPending:
			pending++
		case StatusInProgress:
			inProgress++
		case StatusCompleted:
			completed++
		case StatusFailed:
			failed++
		}
	}

	return
}
