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
//     status 可选值：pending / in_progress / blocked / completed / archived
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
	taskToolDesc = `创建、更新、查询、删除、归档、重开任务。
- action: 操作类型，必填。可选值：create/update/get/list/delete/archive/reopen
- id: 任务ID（用于 update/get/delete/archive/reopen）
- title: 任务标题（用于 create）
- description: 任务描述（用于 create）
- status: 任务状态（pending/in_progress/blocked/completed/archived）`
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
