package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
)

// TaskTool 统一的任务管理工具
type TaskTool struct {
	taskList *agent.TaskList
}

func NewTaskTool(taskList *agent.TaskList) *TaskTool {
	return &TaskTool{taskList: taskList}
}

func (t *TaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "task",
		Desc: "Manage tasks: create, update, get, list, or delete tasks.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     "Action: 'create', 'update', 'get', 'list', 'delete'",
				Required: true,
			},
			"id": {
				Type:     schema.String,
				Desc:     "Task ID (required for create, update, get, delete)",
				Required: false,
			},
			"title": {
				Type:     schema.String,
				Desc:     "Task title (required for create)",
				Required: false,
			},
			"description": {
				Type:     schema.String,
				Desc:     "Task description (required for create)",
				Required: false,
			},
			"status": {
				Type:     schema.String,
				Desc:     "Task status: pending, in_progress, completed, failed (for update or list filter)",
				Required: false,
			},
		}),
	}, nil
}

func (t *TaskTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input struct {
		Action      string `json:"action"`
		ID          string `json:"id,omitempty"`
		Title       string `json:"title,omitempty"`
		Description string `json:"description,omitempty"`
		Status      string `json:"status,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	switch input.Action {
	case "create":
		return t.createTask(input.ID, input.Title, input.Description)
	case "update":
		return t.updateTask(input.ID, input.Status)
	case "get":
		return t.getTask(input.ID)
	case "list":
		return t.listTasks(input.Status)
	case "delete":
		return t.deleteTask(input.ID)
	default:
		return "", fmt.Errorf("unknown action: %s", input.Action)
	}
}

func (t *TaskTool) createTask(id, title, desc string) (string, error) {
	if id == "" || title == "" || desc == "" {
		return "", fmt.Errorf("id, title, and description are required")
	}

	task, err := t.taskList.CreateTask(id, title, desc)
	if err != nil {
		return "", err
	}

	result, _ := json.Marshal(task)
	return string(result), nil
}

func (t *TaskTool) updateTask(id, status string) (string, error) {
	if id == "" || status == "" {
		return "", fmt.Errorf("id and status are required")
	}

	if err := t.taskList.UpdateTaskStatus(id, agent.TaskStatus(status)); err != nil {
		return "", err
	}

	task, _ := t.taskList.GetTask(id)
	result, _ := json.Marshal(task)
	return string(result), nil
}

func (t *TaskTool) getTask(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("id is required")
	}

	task, err := t.taskList.GetTask(id)
	if err != nil {
		return "", err
	}

	result, _ := json.Marshal(task)
	return string(result), nil
}

func (t *TaskTool) listTasks(status string) (string, error) {
	var tasks []*agent.Task

	if status != "" {
		tasks = t.taskList.ListTasksByStatus(agent.TaskStatus(status))
	} else {
		tasks = t.taskList.ListTasks()
	}

	total, pending, inProgress, completed, failed := t.taskList.GetProgress()
	result := map[string]interface{}{
		"tasks": tasks,
		"progress": map[string]int{
			"total":       total,
			"pending":     pending,
			"in_progress": inProgress,
			"completed":   completed,
			"failed":      failed,
		},
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

func (t *TaskTool) deleteTask(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("id is required")
	}

	if err := t.taskList.DeleteTask(id); err != nil {
		return "", err
	}

	return fmt.Sprintf("Task '%s' deleted", id), nil
}

var _ tool.InvokableTool = (*TaskTool)(nil)
