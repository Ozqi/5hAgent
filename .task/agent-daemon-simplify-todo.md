# Agent Daemon 简化 TODO

目标：持续做删减和可读性整理，不新增大功能。每轮只处理少量低风险项，完成后更新本文。

## 1. 删减过度设计

- [x] 重新检查 `internal/systemd` 是否需要保留完整 daemon/process/supervisor 抽象。
  - 已删：`manual.request` 事件入口没有真实事件源，已从 dispatch 和文档移除。
  - 已删：`ProcessNew` 状态没有创建或判断使用，进程创建后直接进入 `running`。
  - 已删：`Turns/MaxTurns` 没有真实 turn 计数回填，属于假生效退出条件；退出条件只保留 `Condition`。
  - 已删：`AgentProcess.LastEvent` 只赋值不读取，会放大进程快照和 decision 输入。
  - 已删：`DecisionEscalate` 是空动作占位，没有真实处理路径。
  - 已删：`DecisionWait` 是 no-op action；不触发 decision 或返回非法 action 已足够表达不动作。
  - 已删：`Policy.HighRiskWait` 只是重复描述 `Event.Risk == "high"` 硬规则，不再放进 decision 输入。
  - 已删：`process.stopped` 外部事件入口没有真实 emitter。
  - 已删：`Event.Risk` 没有真实生产者，风险控制应放在工具权限或调用方策略，不放在 systemd 事件结构里。
  - 已删：`DecisionStop/stop_agent` 在当前串行 daemon 中没有真实价值。
  - 已删：`stalledAfter/--stalled-after/LastActiveAt` 没有真实心跳支撑。
  - 已删：`Deadline/StartTimer/timer.tick/ProcessStopped` 没有当前生产路径；进程结束由 `RunProcess` 返回驱动。
  - 已删：`process.start` 外部事件入口没有当前调用方；daemon 只通过 `task.created` 启动 AgentProcess。
  - 已删：`DecisionCaller/Decision/ParseDecision/retry/max-retry` 没有 `task.failed` 生产者，当前 daemon 改为纯确定性 task supervisor。
  - 已删：`SkillRef/PromptSpec.Skills` 没有真实生产者。
  - 已删：`PromptSpec/ExitSpec` 两层壳，`ProcessSpec` 压平为 `SystemPrompt/ExitCondition`。
  - 已删：`ProcessEventPayload` 重复携带 report/worklog；路径已在 `AgentProcess` 上回填。
  - 已删：`TaskCreatedPayload.FileEvent` 没有消费者；事件 ID 已足够追踪来源。
- 优先删掉当前没有真实调用方或只是 OS 类比的类型、字段、事件和 helper。
- 保留最小链路：task 事件 -> 启动 Agent -> 写 report/worklog -> 更新 task 状态。
- 暂不做并发多 Agent；明确当前只支持串行执行。

### 当前保留项

- [x] 保留 `SourceTask`：它承载 task id/title/event id，供 runtime 写 report/worklog 并回写 task 状态；不是重复 `ProcessSpec`。

## 2. 排查冲突设计

- [x] 检查 Runtime 已有 session 持久化和 daemon/sys.session 工具是否职责重复。
- [x] 明确默认策略：Runtime 负责已有 session 能力；daemon 不再额外定义一套“AgentProcess 持久化落盘”语义。
  - 结论：`sys.session` 不再注册为 LLM 可见工具；`BindSession/SaveSession/DropSession` 保留在 `internal/context`。
  - 已删：`internal/tools/session_tool.go` 是未注册旧实现，已删除；session 能力保留在 `internal/context`。
- [x] 检查类似冲突：context 持久化、process report、task report、worklog、IPC artifact。
  - context 持久化：保留 `internal/context`，daemon 不再暴露 `sys.session`。
  - process report：保留 `Runtime.RunProcess` 写 `<task-id>.<process-id>.<timestamp>.md`。
  - task report：保留 `RunTaskOnce` 写 `<task-id>.md`，不和 process report 合并。
  - worklog：保留 `.5hagent/agents/<agent>/logs/`，只记录执行流和工具事件。
  - IPC artifact：保留 `ipctypes.Message.Artifact` 作为路径字段；后续若强化安全，再加路径校验，不在 systemd 里理解业务内容。
- [x] 对每个冲突点给出“保留谁、删除谁、迁移到哪里”的结论。

## 3. 增强代码注释

- [x] 给 `internal/systemd` 的大段逻辑补模块分区注释。
  - 已补：数据契约、调度器状态、主循环、事件解析、事件源、进程执行、IPC。
- [x] 给 `internal/runtime`、`internal/agent` 的大段逻辑补模块分区注释。
  - `internal/runtime` 已补：Runtime 配置、初始化、Systemd 适配、headless task、内部 helper、报告渲染。
  - `internal/agent` 已补：核心状态、Runtime 注入点、ReAct 辅助状态、主循环、访问器。
- [x] 对复杂函数补步骤注释，例如：收事件 -> 解析 payload -> 启动进程 -> 写报告 -> 更新 task。
- 注释要解释职责边界，不复述代码。
- 中文优先；代码标识符和 API 名保持原文。

## 验收

- TODO 中的删减项都有明确结论，不留下“可能需要”。
- 代码仍保持最小正确链路可运行。
- `go test ./internal/systemd ./internal/runtime ./internal/agent` 通过。
- 如果改 daemon wiring，额外在 `/Users/bytedance/Proj/5hWorkSpace` 做一次真实 daemon 冒烟。
