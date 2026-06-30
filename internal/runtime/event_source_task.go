// event_source_task.go - task.md 到 Agent Systemd 事件的适配层
// 功能：监听项目 task 文件变化，把 in_progress/pending 任务转换为 task.created 事件。
// 调用方：后续 Agent Systemd 启动入口；调度核心只接收 systemd.Event，不直接读取 task 包。
// 全局变量：无。
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lzq/5hAgent/internal/systemd"
	"github.com/lzq/5hAgent/internal/task"
)

// TaskSpecBuilder 把任务转换为 AgentProcess 启动规格。
type TaskSpecBuilder func(t *task.Task) (systemd.ProcessSpec, error)

// TaskFileEventSource 把 task.md 文件变化解析成 task.created 事件。
type TaskFileEventSource struct {
	file  *systemd.FileEventSource // 底层文件 watcher
	list  *task.TaskList           // 任务列表
	build TaskSpecBuilder          // 任务到 ProcessSpec 的转换策略
}

// NewTaskFileEventSource 创建 task.md 事件源。
// 参数：list 是已有任务列表；builder 由上层决定如何把 task 转成 PromptSpec/ExitSpec。
// 调用层级：runtime/systemd 启动入口 -> NewTaskFileEventSource -> AgentSystemd.StartSource。
// 步骤：复用 FileEventSource 监听文件变化；变化后选 in_progress/pending 任务并生成 task.created。
func NewTaskFileEventSource(list *task.TaskList, interval time.Duration, builder TaskSpecBuilder) *TaskFileEventSource {
	path := ""
	if list != nil {
		path = list.Path()
	}
	return &TaskFileEventSource{file: systemd.NewFileEventSource(path, "task.changed", "task", interval), list: list, build: builder}
}

// Next 等待 task.md 变化并返回 task.created 事件。
// 参数：ctx 控制等待生命周期。
// 调用层级：AgentSystemd.StartSource -> TaskFileEventSource.Next -> FileEventSource.Next -> TaskList.ListTasksByStatus。
// 步骤：等待文件变化 -> 读取任务列表 -> 选择 in_progress/pending -> 调 builder 生成 ProcessSpec。
func (w *TaskFileEventSource) Next(ctx context.Context) (systemd.Event, error) {
	if w == nil || w.file == nil || w.list == nil {
		return systemd.Event{}, fmt.Errorf("task file event source path is required")
	}
	if w.build == nil {
		return systemd.Event{}, fmt.Errorf("task spec builder is required")
	}
	for {
		fileEvent, err := w.file.Next(ctx)
		if err != nil {
			return systemd.Event{}, err
		}
		var selected *task.Task
		for _, status := range []task.TaskStatus{task.StatusInProgress, task.StatusPending} {
			tasks := w.list.ListTasksByStatus(status)
			if len(tasks) > 0 {
				selected = tasks[0]
				break
			}
		}
		if selected == nil {
			continue
		}
		spec, err := w.build(selected)
		if err != nil {
			return systemd.Event{}, err
		}
		payload, err := json.Marshal(systemd.TaskCreatedPayload{
			ProcessSpec: spec,
			TaskID:      selected.ID,
			TaskTitle:   selected.Title,
			FileEvent:   fileEvent,
		})
		if err != nil {
			return systemd.Event{}, fmt.Errorf("marshal task event payload: %w", err)
		}
		return systemd.Event{
			ID:        "task." + selected.ID + "." + fileEvent.ID,
			Type:      "task.created",
			Source:    "task",
			Payload:   payload,
			CreatedAt: time.Now().UTC(),
		}, nil
	}
}
