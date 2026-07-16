# Runtime

## 职责

`internal/runtime` 是 TUI、headless、daemon 的共同装配层。它创建配置、任务文件、session、LLM、Agent、本地工具和报告路径。

## 关键文件

| 文件 | 作用 |
| --- | --- |
| [runtime.go](../../internal/runtime/runtime.go) | `Runtime` 初始化、模型切换、task 执行、process 执行 |
| [worklog.go](../../internal/runtime/worklog.go) | headless/process 工作日志 |
| [event_source_task.go](../../internal/runtime/event_source_task.go) | task 文件事件源适配 |
| [cmd/5hagent/main.go](../../cmd/5hagent/main.go) | CLI 子命令入口 |

## 初始化顺序

```text
runtime.New
  -> utils.LoadConfigWithOptions
  -> logger.InitLog
  -> projectRoot / projectDataDir
  -> task.NewTaskList(.5hagent/task.md)
  -> context.NewManager(~/.5hAgent/sessions)
  -> openMessageCtx
  -> llm.NewClient
  -> utils.LoadSystemPromptBase
  -> agent.NewAgent
  -> tools.NewRegistry().Init
  -> RegisterContextTool
  -> bindTools(model.WithTools)
```

启动阶段不启动 MCP stdio server。`/mcp` 只管理配置；远端工具需要未来 lazy/显式连接后注册。

## 入口

| 入口 | 行为 |
| --- | --- |
| `5hagent` | `runtime.New(PromptBase=tui)` -> `cli.LaunchTUI` |
| `5hagent run` | `runtime.New(main)` -> `RunTaskOnce` |
| `5hagent daemon` | `NewInMemory` -> `AgentSystemd` -> `RunProcess` |
| `/run` | TUI 内调用 `RunTasksUntilDone` |
| `/model` | 调 `SwitchModel`，不写 `.env` |

## 输出位置

| 数据 | 路径 |
| --- | --- |
| session | `~/.5hAgent/sessions/*.jsonl` |
| task | `<project>/.5hagent/task.md` |
| headless report | `<project>/.5hagent/reports/<task-id>.md` |
| process report | `<project>/.5hagent/reports/<task-id>.<process-id>.<timestamp>.md` |
| worklog | `<project>/.5hagent/agents/<agent>/logs/*.md` |
| tool failure stats | `~/.5hAgent/tool-stats/failures.jsonl` 和 `<project>/.5hagent/agents/<agent>/logs/tool-failures.jsonl` |

## Runtime Hooks

项目可选配置 `<project>/.5hagent/hooks.json`。首版只监听稳定工具事件：

- `tool_start`
- `tool_end`
- `tool_error`

hook 异步执行，默认 5s 超时，失败只写 warning，不阻塞主 Agent。事件 payload 通过 stdin 传入 JSON，包含 `event/tool/args_summary/result_summary/error/workspace/session_id/time`。

## 边界

- runtime 不执行 ReAct 细节；那是 `internal/agent`。
- runtime 不渲染 TUI；那是 `internal/cli`。
- runtime 不把 MCP 作为启动阻塞项。
- `MemoryContext` 用于 systemd/process；默认不绑定 session，但保留 store。
