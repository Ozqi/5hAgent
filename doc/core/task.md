# Task

## 职责

`internal/task` 管理项目 `.5hagent/task.md`。它是 headless/daemon 的任务真源。

## 文件

| 文件 | 作用 |
| --- | --- |
| [tasklist.go](../../internal/task/tasklist.go) | Task 解析、CRUD、Markdown 持久化 |
| [task_actions.go](../../internal/task/task_actions.go) | action 分发 |
| [task_tool.go](../../internal/tools/task_tool.go) | LLM 工具封装 |
| [task.go](../../internal/commands/task.go) | `/task` 命令 |

## 状态

```text
pending -> in_progress -> completed
                         -> failed
completed -> archived
archived  -> pending/in_progress
```

## 文件边界

`task.md` 中 managed block 是任务真源：

```text
<!-- 5hagent:tasks:start -->
...
<!-- 5hagent:tasks:end -->
```

模板区不要被工具随意重写。

## 工具 action

| action | 说明 |
| --- | --- |
| `list` | 列任务和 progress |
| `get` | 取单个任务 |
| `create` | 创建任务 |
| `update` | 改标题/描述/状态 |
| `delete` | 删除任务 |
| `archive` | 归档任务 |
| `reopen` | 从归档恢复 |
