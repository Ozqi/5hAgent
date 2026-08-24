# Runtime

> 由 Claude Fable 5 于 2026-08-23 阅读 `internal/runtime/runtime.go`、`internal/runtime/daemon_session.go`、`internal/tools/registry.go` 与 `cmd/walle/main.go` 后更新。
> 覆盖范围：交互 Runtime 装配、模型切换、daemon session、通用 AgentProcess 执行与持久化边界。

## 职责

`internal/runtime` 是 Agent 装配层。它创建配置、session、LLM、Agent、本地工具与 daemon session，也保留 `internal/systemd.ProcessRunner` 所需的通用 process 执行能力。

Runtime 不再内置 TaskList、文件任务入口、task watcher 或 task report/status。

## 关键文件

| 文件 | 作用 |
| --- | --- |
| [runtime.go](../../internal/runtime/runtime.go) | `Runtime` 初始化、模型切换、通用 process 执行 |
| [daemon_session.go](../../internal/runtime/daemon_session.go) | 可 attach 的长驻交互 Agent 与事件回放 |
| [worklog.go](../../internal/runtime/worklog.go) | 通用 process 工作日志 |
| [cmd/walle/main.go](../../cmd/walle/main.go) | 默认 TUI 与 daemon 入口 |

## 初始化顺序

```text
runtime.New
  -> utils.LoadConfigWithOptions
  -> logger.InitLog
  -> projectRoot / projectDataDir
  -> context.NewManager(~/.walle/sessions)
  -> openMessageCtx
  -> llm.NewClient
  -> utils.LoadSystemPromptBase
  -> agent.NewAgent
  -> tools.NewRegistry().Init(skillMgr)
  -> RegisterContextTool
  -> bindTools(model.WithTools)
```

启动阶段不启动 MCP stdio server。`/mcp` 只管理配置；远端工具需要显式连接后注册。

## 入口

| 入口 | 行为 |
| --- | --- |
| `walle` | 连接 `supervisor.sock`，attach `interactive`，必要时自动启动 daemon |
| `walle daemon` | 固定托管一个可 attach 的交互 Agent；无 `--poll`、`--interactive` |
| `walle ps` | 查询 control socket 的 `ProcessSnapshot` |
| `walle attach <id>` | attach 交互进程，或跟随通用 process worklog |
| `/model` | 调 `SwitchModel`，只更新当前 Runtime 内存模型 |
| `/stop` | 取消当前交互执行 |

## 通用 ProcessRunner

`Runtime.RunProcess` 接收 `systemd.AgentProcess`，根据 `ProcessSpec` 启动一轮 Agent，写 process worklog/report，并把路径回填到进程对象。它不依赖任务 ID、任务标题或任务状态。

`internal/systemd` 的启动事件固定为 `process.start`，payload 为 `ProcessStartPayload{process_spec}`。`AgentProcess` 和 `ProcessSnapshot` 使用 `Name`，不携带 `SourceTask`、`TaskID` 或 `TaskTitle`。

## 输出位置

| 数据 | 路径 |
| --- | --- |
| session | `~/.walle/sessions/*.jsonl` |
| 用户默认设置 | `~/.walle/settings.json` |
| 运行进程状态 | `~/.walle/run/supervisor.sock` 的 `ProcessSnapshot` |
| 通用 process report | `<project>/.walle/reports/<process-id>.<timestamp>.md` |
| process worklog | `<project>/.walle/agents/<process-id>/logs/*.md` |
| tool failure stats | `~/.walle/tool-stats/failures.jsonl` 和项目级 `tool-failures.jsonl` |

## Runtime Hooks

项目可选配置 `<project>/.walle/hooks.json`。当前监听：`tool_start`、`tool_end`、`tool_error`。hook 异步执行，默认 5s 超时，失败只写 warning，不阻塞主 Agent。

## 边界

- Runtime 不执行 ReAct 细节；那是 `internal/agent`。
- Runtime 不渲染 TUI；那是 `internal/tui`。
- Runtime 不维护 TaskList 或固定任务管理协议。
- 任务管理未来可由 Skill、MCP 或外置动态工具提供，本次没有新增接口。
- Runtime 不把 MCP 作为启动阻塞项。
