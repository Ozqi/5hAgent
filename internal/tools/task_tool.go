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
		Name: "task.task",
		Desc: "Manage tasks: create, update, get, list, delete, archive, or reopen tasks.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     "Action: 'create', 'update', 'get', 'list', 'delete', 'archive', 'reopen'",
				Required: true,
			},
			"id": {
				Type:     schema.String,
				Desc:     "Task ID (required for create, update, get, delete, archive, reopen)",
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
				Desc:     "Task status: pending, in_progress, blocked, completed, archived (for update, list filter, or reopen target status)",
				Required: false,
			},
		}),
	}, nil
}

func (t *TaskTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input agent.TaskActionRequest

	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	result, err := agent.ExecuteTaskAction(t.taskList, input)
	if err != nil {
		return "", err
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

var _ tool.InvokableTool = (*TaskTool)(nil)
