// task_tool.go - 任务管理工具
package tools

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/task"
)

type TaskTool struct{ taskList *task.TaskList }

func NewTaskTool(t *task.TaskList) *TaskTool { return &TaskTool{taskList: t} }

func (t *TaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "task.task",
		Desc: "Manage tasks: create, update, get, list, delete, archive, or reopen tasks.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action":      {Type: schema.String, Desc: "Action: create/update/get/list/delete/archive/reopen", Required: true},
			"id":          {Type: schema.String, Desc: "Task ID (for update/get/delete/archive/reopen)"},
			"title":       {Type: schema.String, Desc: "Task title (for create)"},
			"description": {Type: schema.String, Desc: "Task description (for create)"},
			"status":      {Type: schema.String, Desc: "Status: pending/in_progress/blocked/completed/archived"},
		}),
	}, nil
}

func (t *TaskTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input task.TaskActionRequest
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", err
	}
	result, err := task.ExecuteTaskAction(t.taskList, input)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

var _ tool.InvokableTool = (*TaskTool)(nil)
