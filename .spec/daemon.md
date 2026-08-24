# Daemon / Systemd Spec

## 职责

`internal/systemd` 是标准库实现的通用本机 process 调度和控制核心。它管理 `AgentProcess`、事件队列、去重、进程快照和 Unix Socket NDJSON 控制通道。

`walle daemon` 固定托管一个 `DaemonSession` 交互 Agent；不再内置 TaskList watcher，也没有 `--poll` 或 `--interactive` 分支。

## 覆盖文件

| 文件 | 职责 |
| --- | --- |
| `internal/systemd/systemd.go` | AgentProcess、ProcessSpec、事件、调度循环、通用 FileEventSource。 |
| `internal/systemd/control.go` | supervisor socket、NDJSON 控制协议、ProcessClient。 |
| `internal/runtime/daemon_session.go` | interactive process 的 Runtime 适配层。 |

## 数据契约

| 类型 | 关键字段 | 说明 |
| --- | --- | --- |
| `ProcessSpec` | `SystemPrompt`、`ExitCondition` | 启动 AgentProcess 的最小规格。 |
| `Event` | `ID`、`Type`、`Source`、`ProcessID`、`Payload`、`CreatedAt` | 调度器事实输入。 |
| `ProcessStartPayload` | `ProcessSpec` | `process.start` payload，严格解析。 |
| `AgentProcess` | `ID`、`Name`、`State`、`Spec`、report/worklog path、cancel | 内存进程记录。 |
| `ProcessSnapshot` | `ID`、`Name`、状态、workspace、model、session、turn | 跨进程只读快照。 |
| `ProcessEvent` | `Seq`、`Type`、文本与工具/状态字段 | daemon 推给 attached TUI 的事件。 |

不定义 `SourceTask`、`TaskID` 或 `TaskTitle`。

## 调度流程

```text
EventSource.Next -> AgentSystemd.Emit -> Run.nextEvent
    -> dispatch(process.start) -> goroutine RunProcess
    -> ProcessRunner.RunProcess
    -> Emit(process.exited/process.failed)
    -> applyEvent 更新进程终态
```

- `process.start` 只负责启动进程，不阻塞后续事件消费。
- `process.exited/process.failed` 按 `ProcessID` 更新进程。
- 非空事件 ID 只入队一次。
- `decodeStrict` 拒绝 payload 未声明字段。

## 控制通道

- Socket：`~/.walle/run/supervisor.sock`。
- 协议：单连接 NDJSON。
- 短连接：`list`。
- 长连接：`attach` 后可发送 `input`、`stop`、`detach`。
- Attach 握手：`attached` -> history `event` -> `ready` -> live `event`。

## Interactive session

- `Snapshot` 返回固定 ID/Name `interactive`、状态、workspace、model、session、turn。
- `Attach` 原子复制历史并注册订阅者。
- 普通输入要求 session 空闲，异步调用 `Agent.RunStream`。
- `/model`、`/provider` 与其他 slash command 由 `DaemonSession` 处理。
- socket 断开只解除订阅；`stop` 才取消当前 run。

## 状态边界

- 进程表、事件队列和 seen 去重表在 `AgentSystemd` 内存中。
- interactive 历史在 `DaemonSession` 内存中。
- 通用 process report/worklog 路径可保存在 `AgentProcess`，但不带 task 状态语义。
- control server 固定使用 `supervisor.sock`。

## 不变量

- `internal/systemd` 只依赖标准库和本包类型。
- `ProcessSpec` 只放启动 prompt 和退出条件。
- `ProcessSnapshot` 用 `Name` 展示进程，不映射任务标题。
- `detach` 是离开观察通道，`stop` 是取消执行。

## 禁止

- 在 `internal/systemd` 引入 `task` 或 Runtime 业务包。
- 恢复 `.walle/task.md` watcher、`task.created` 或源任务状态回写。
- 恢复多个 `daemon-*.sock` 的发现式扫描。
