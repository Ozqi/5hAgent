# Agentd

> 由 Claude Fable 5 于 2026-08-26 阅读 `internal/agentd/agentd.go`、`internal/agentd/control.go`、`internal/runtime/runtime.go`、`internal/runtime/daemon_session.go` 与 `cmd/walle/*.go` 后更新。
> 覆盖范围：通用 AgentProcess 调度、Unix Socket 控制面和 workspace interactive Runtime 托管。

## 当前结论

`internal/agentd` 保留通用 process 调度能力；`walle daemon` 是用户级 supervisor，通过 control socket 托管多个可 attach 的 interactive Runtime。Runtime 不再内置 TaskList 驱动的 supervisor、文件 watcher 或 task 状态回写。

| 对象 | 职责 |
| --- | --- |
| `walle daemon` | 启动 supervisor/control socket，并按 `open` 请求创建或续接 interactive Runtime。 |
| `internal/agentd.Agentd` | 通用事件队列、进程表、`process.start` 调度。 |
| `DaemonSession` | 把一个 Runtime 暴露为一个 interactive process。 |
| `walle` / TUI | 自动启动或复用 daemon，再 open 当前 workspace interactive Runtime 并 attach。 |
| `walle -c` | 显式续接当前 workspace 最近 Runtime 或最近 session。 |
| `walle ps/attach` | 查询与连接已有进程，不隐式创建 Runtime。 |

## 当前启动链路

```text
walle
  -> open ~/.walle/run/supervisor.sock with workspace + continue=false
  -> missing socket: start current binary with "daemon"
  -> retry open
  -> daemon creates a new Runtime(ProjectDir=cwd, ContinueLast=false)
  -> attach returned process id
  -> LaunchAttachedTUI

walle -c
  -> open ~/.walle/run/supervisor.sock with workspace + continue=true
  -> daemon attaches latest matching Runtime or creates Runtime(ContinueLast=true)

walle daemon
  -> agentd.New
  -> StartControlServer
  -> wait for open/list/attach requests
```

`daemon` 不接受 `--poll` 或 `--interactive`；它是 supervisor，不是某一个固定 interactive Agent 宿主。

## 通用 Process 模型

| 类型 | 关键字段 | 说明 |
| --- | --- | --- |
| `ProcessSpec` | `SystemPrompt`、`ExitCondition` | 启动 AgentProcess 的最小规格。 |
| `ProcessStartPayload` | `ProcessSpec` | `process.start` 的严格 payload。 |
| `AgentProcess` | `ID`、`Name`、`State`、`Spec`、report/worklog path | 内存进程记录。 |
| `ProcessSnapshot` | `ID`、`Name`、状态、workspace、model、session、turn | 跨进程只读快照。 |

不存在 `SourceTask`、`TaskID` 或 `TaskTitle`。启动事件是 `process.start`，不是任务事件。

## 控制面

固定 socket：

```text
~/.walle/run/supervisor.sock
```

NDJSON 协议：

- `open`：按 workspace/session/continue/model 覆盖创建或打开 interactive Runtime。
- `list`：列出当前 daemon 已托管的 interactive Runtime 和通用 process。
- `attach`：连接指定 process。
- `input`：向 attached interactive Runtime 发送用户输入。
- `stop`：取消目标 Runtime 当前 run。
- 客户端断开使用内部 `detach`。

Attach 握手顺序固定为：`attached` -> history `event` -> `ready` -> live `event`。

## 持久化与状态

- process table、事件队列和去重表在 `Agentd` 内存中。
- interactive registry 在 daemon 内存中；daemon 退出后 Runtime 消失，但 session JSONL 保留。
- 每个 `DaemonSession` 自己维护事件历史；不同 interactive Runtime 的历史隔离。
- session 写入 `~/.walle/sessions/*.jsonl`。
- 通用 process runner 可写 report/worklog，但没有 task report/status 语义。
- control server 启动时清理旧 `daemon-*.sock`，固定使用 `supervisor.sock`。

## 边界

- `internal/agentd` 只依赖标准库和本包类型。
- daemon control 不执行工具或调用 LLM；工具和 LLM 只在 Runtime/Agent 内。
- 默认 `walle` 创建新的 Runtime；自动续接只属于 `-c/--continue`、`--session` 或显式 `attach`。
- `detach` 只断开观察；`stop` 才取消当前执行。
- 任务管理可由 Skill、MCP 或外置动态工具提供，不进入固定 daemon 协议。
