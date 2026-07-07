# Slash Commands

## 职责

`internal/commands` 保存可复用的命令处理；TUI 的 `submit()` 决定何时调用它们。

## 文件

| 文件 | 命令 |
| --- | --- |
| [task.go](../../internal/commands/task.go) | `/task` |
| [skill.go](../../internal/commands/skill.go) | `/skill` |
| [compress.go](../../internal/commands/compress.go) | `/compress` |
| [mcp.go](../../internal/commands/mcp.go) | `/mcp` |

## TUI 内部命令

这些不在 `internal/commands`：

- `/session`：直接操作 `context.Manager`。
- `/model`：调用 `runtime.SwitchModel`。
- `/run`：调用 `Runtime.RunTasksUntilDone`。
- `/stop`：取消当前 TUI run context。

## 边界

- Slash 命令不经过 LLM。
- 命令错误以 system entry 显示。
- 长任务只由 `/run` 进入文件任务执行链路。
