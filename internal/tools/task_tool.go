// task_tool.go - 任务管理工具
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: task
// Desc: 创建、更新、查询、删除、归档、重开任务。
//
//	任务用于将复杂工作拆解为可管理的小单元。
//
// Input Parameters:
//
//   - action      (string, required) : 操作类型
//
//   - id          (string, optional) : 任务ID（用于 update/get/delete/archive/reopen）
//
//   - title       (string, optional) : 任务标题（用于 create）
//
//   - description (string, optional) : 任务描述（用于 create）
//
//   - status      (string, optional) : 任务状态
//
//     action 可选值：
//
//   - create   → 新建任务（需要 title）
//
//   - update   → 更新任务（需要 id）
//
//   - get      → 获取单个任务详情（需要 id）
//
//   - list     → 列出任务（支持状态过滤）
//
//   - delete   → 删除任务（需要 id）
//
//   - archive  → 归档任务（需要 id）
//
//   - reopen   → 重开任务（需要 id）
//
//     status 可选值：pending / in_progress / blocked / completed / archived / failed
//
// Error Scenarios (LLM Hints):
//   - MISSING 'action'            → 必须提供 action
//   - MISSING 'id' (for update/get/delete/archive/reopen)
//     → 这些操作需要指定任务 id
//   - MISSING 'title' (for create) → create 操作需要 title
//   - task not found              → 指定的任务 ID 不存在
//   - invalid status              → status 必须是有效值之一
//
// Tips:
//   - 复杂任务先用 task create 拆解，再用 subtask 关联
//   - 已归档任务可通过 reopen 重新激活
//
// ============================================================
package tools

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/task"
)

// --- LLM 描述常量 ---
const (
	taskToolName = "task.task"
	taskToolDesc = `Manage the local task list. Always send JSON object arguments.
Tool name is always task.task. Never call task.create, task.update, task.delete, task.list, or task.get.
Allowed action values only: create, update, get, list, delete, archive, reopen.
Do NOT invent action names such as finish, done, complete, start, or show.

Required fields by action:
- create: action, id, title, description
- update: action, id, status
- get/delete/archive: action, id
- reopen: action, id, optional status (pending or in_progress)
- list: action, optional status

Examples:
- Create: {"action":"create","id":"fix-tui-wrap","title":"Fix TUI wrapping","description":"Prevent content from overlapping the right panel"}
- Mark completed: {"action":"update","id":"fix-tui-wrap","status":"completed"}
- Delete: {"action":"delete","id":"fix-tui-wrap"}
- List active: {"action":"list","status":"in_progress"}`
	taskToolErrors = `MISSING 'action': 必须提供 action
MISSING 'id': update/get/delete/archive/reopen 操作需要指定任务 id
MISSING 'title': create 操作需要 title
task not found: 指定的任务 ID 不存在
invalid status: status 必须是有效值之一`
	taskToolTips = `复杂任务先用 task create 拆解
已归档任务可通过 reopen 重新激活`
)

type TaskTool struct{ taskList *task.TaskList }

func (t *TaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: taskToolName,
		Desc: taskToolDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action":      {Type: schema.String, Desc: "Required. One of: create, update, get, list, delete, archive, reopen. To finish a task use action=update and status=completed.", Required: true, Enum: []string{"create", "update", "get", "list", "delete", "archive", "reopen"}},
			"id":          {Type: schema.String, Desc: "Task ID. Required for create/update/get/delete/archive/reopen. Example: fix-tui-wrap"},
			"title":       {Type: schema.String, Desc: "Task title. Required for create."},
			"description": {Type: schema.String, Desc: "Task description. Required for create."},
			"status":      {Type: schema.String, Desc: "Task status. Required for update. One of: pending, in_progress, blocked, completed, archived, failed.", Enum: []string{"pending", "in_progress", "blocked", "completed", "archived", "failed"}},
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
