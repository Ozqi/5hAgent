# Agent Systemd

> 由 Claude Fable 5 于 2026-08-23 阅读 `internal/systemd/systemd.go`、`internal/systemd/control.go`、`internal/runtime/runtime.go`、`internal/runtime/daemon_session.go` 与 `cmd/walle/*.go` 后更新。
> 覆盖范围：通用 AgentProcess 调度、Unix Socket 控制面和当前固定交互 daemon。

## 当前结论

`internal/systemd` 保留通用 process 调度能力；`walle daemon` 当前只托管一个可 attach 的交互 Agent。Runtime 不再内置 TaskList 驱动的 supervisor、文件 watcher 或 task 状态回写。

| 对象 | 职责 |
| --- | --- |
| `walle daemon` | 创建 Runtime、`DaemonSession` 和固定 control socket |
| `internal/systemd.AgentSystemd` | 通用事件队列、进程表、`process.start` 调度 |
| `DaemonSession` | 把一个 Runtime 暴露为 `interactive` process |
| `walle` / TUI | 自动启动或复用 daemon，再 attach |
| `walle ps/attach` | 查询与连接进程 |

## 当前启动链路

```text
walle
  -> attach ~/.walle/run/supervisor.sock / interactive
  -> missing: start current binary with "daemon"
  -> retry attach
  -> LaunchAttachedTUI

walle daemon
  -> runtime.New(PromptBase=tui)
  -> systemd.New
  -> runtime.NewDaemonSession
  -> StartControlServer
```

`daemon` 不接受 `--poll` 或 `--interactive`；它始终是 attachable interactive Agent 宿主。

## 通用 Process 模型

| 类型 | 关键字段 | 说明 |
| --- | --- | --- |
| `ProcessSpec` | `SystemPrompt`、`ExitCondition` | 启动 AgentProcess 的最小规格 |
| `ProcessStartPayload` | `ProcessSpec` | `process.start` 的严格 payload |
| `AgentProcess` | `ID`、`Name`、`State`、`Spec`、report/worklog path | 内存进程记录 |
| `ProcessSnapshot` | `ID`、`Name`、状态、workspace、model、session、turn | 跨进程只读快照 |

不存在 `SourceTask`、`TaskID` 或 `TaskTitle`。启动事件是 `process.start`，不是任务事件。

## 控制面

固定 socket：

```text
~/.walle/run/supervisor.sock
```

NDJSON 协议保留：

- `list`
- `attach`
- `input`
- `stop`
- 客户端断开使用内部 `detach`

Attach 握手顺序固定为：`attached` -> history `event` -> `ready` -> live `event`。

## 持久化与状态

- process table、事件队列和去重表在 `AgentSystemd` 内存中。
- interactive 事件历史在 `DaemonSession` 内存中；daemon 退出后丢失。
- session 写入 `~/.walle/sessions/*.jsonl`。
- 通用 process runner 可写 report/worklog，但没有 task report/status 语义。
- control server 启动时清理旧 `daemon-*.sock`，固定使用 `supervisor.sock`。

## 边界

- `internal/systemd` 只依赖标准库和本包类型。
- daemon control 不执行工具或调用 LLM，只把输入路由给 `InteractiveProcess`。
- `detach` 只断开观察；`stop` 才取消当前执行。
- 任务管理可由 Skill、MCP 或外置动态工具提供，不进入固定 daemon 协议。
