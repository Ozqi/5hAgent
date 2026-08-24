# Runtime 对外接口

一句话：Runtime 是独立工作实例；当前 daemon 暴露单个 supervisor 的进程快照，尚未形成跨 workspace 的 N runtime 统一表。

设计事实源：`.doc/runtime-daemon-boundary.md`、`.doc/runtime-interface.md`。

## 当前判断

| 对象 | 结论 |
| --- | --- |
| Runtime | 可直接运行；负责装配 Agent/Context/Tools/LLM |
| daemon | 可选守护进程；当前负责单个 supervisor 的进程表、socket 和交互路由 |
| CLI | 当前默认启动或复用 `walle daemon`，再通过 socket attach |
| TUI | client；只输入和展示事件 |
| doc 分工 | `.doc/` 写设计，`.TODO/` 写任务，`doc/` 写对外稳定文档 |

## 接口分级

| 接口 | 分级 |
| --- | --- |
| `New` | 稳定入口 |
| `SwitchModel` | 稳定入口；不改 `.env` |
| `RunProcess` | 已实现；属于 Runtime 与 AgentSystemd 的通用 process 适配边界 |
| `NewDaemonSession` | 已实现；包装一个可 attach 的交互 Runtime |
| `RecordToolEvent` | 已实现；仅记录交互模式工具失败，不作为通用事件总线 |
| `ProcessSnapshot` / `ListProcesses` / `AttachProcess` | 已实现 supervisor 控制面；覆盖当前运行进程查询和 attach 路由 |
| `Runtime` 公开字段 | 待收口 |

运行态查询统一以 `supervisor.sock` 暴露的 `ProcessSnapshot` 为准。跨 workspace 的 N runtime 发现继续扩展 supervisor 控制面，不再维护无人消费的磁盘快照。

## 下一步

- [ ] 扩展 `ProcessSnapshot` 和 supervisor 查询语义，定义 runtime 身份、workspace、session、状态和过期边界。
- [ ] 实现 N runtime 的跨 workspace 发现与汇总；保持 supervisor 为唯一 process registry。
