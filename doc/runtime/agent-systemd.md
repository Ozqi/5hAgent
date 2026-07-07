# Agent Systemd

## 职责

Agent Systemd 是最小 task supervisor。它监听事件，串行启动 `AgentProcess`，由 runtime 执行任务并写 report/worklog。

## 当前边界

- 串行多 AgentProcess，不并发。
- `internal/systemd` 只依赖标准库。
- `ProcessSpec` 只保留 `SystemPrompt` 和 `ExitCondition`。
- task 来源放在 `AgentProcess.SourceTask`，不塞进 `ProcessSpec`。
- context 默认是进程内存；持久化由 Agent 显式写文件或 runtime session 能力处理。
- decision/LLM sudo 逻辑当前不在默认 daemon 路径。

## 关键文件

| 文件 | 作用 |
| --- | --- |
| [systemd.go](../../internal/systemd/systemd.go) | 事件、进程表、调度循环、IPC |
| [runtime.go](../../internal/runtime/runtime.go) | `RunProcess` 执行 AgentProcess |
| [event_source_task.go](../../internal/runtime/event_source_task.go) | `.5hagent/task.md` -> `task.created` |
| [ipctypes/ipc.go](../../internal/ipctypes/ipc.go) | IPC 消息结构 |

## 调度链路

```text
5hagent daemon
  -> systemd.New
  -> NewTaskFileEventSource
  -> EmitCurrentTask
  -> AgentSystemd.Run
  -> dispatch task.created
  -> RunProcess
  -> runtime.RunProcess
  -> report/worklog
  -> update source task status
```

## 保留的语义

| 项 | 当前实现 |
| --- | --- |
| task 状态闭环 | 成功 `completed`；失败 `failed` |
| report 命名 | `<task-id>.<process-id>.<timestamp>.md` |
| worklog | `.5hagent/agents/<process-id>/logs/` |
| IPC | `Send/Recv` 短消息；字段为 `from/to/summary/artifact` |
| 去重 | `timer.tick` 不进入 seen；进程结束清理相关 key |

## 不要恢复

- `manual.request`、`process.start` 外部入口。
- `ProcessNew`、`DecisionWait`、`DecisionEscalate` 空状态。
- `PromptSpec/ExitSpec` 双层壳。
- 启动时同步 MCP。
