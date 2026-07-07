# TODO.messages.md - Agent Systemd TODO 复审 (r6)

> 2026-06-30 复测后更新。核对范围：`5hagent daemon --poll 1s` 在 `/Users/bytedance/Proj/5hWorkSpace` 的真实运行结果、`internal/systemd/systemd.go`、`internal/runtime/runtime.go`、`internal/runtime/event_source_task.go`、`doc/runtime/agent-systemd.md`。
>
> 结论：Agent Systemd 当前是“最小 daemon 调度骨架可用”，不是“完备多 Agent 调度框架”。它可以从 `.5hagent/task.md` 启动 AgentProcess，写 process report/worklog，并在同一 daemon 生命周期内串行启动 `agent-1`、`agent-2`。r6 P0 已完成：任务状态闭环、task trace、report 不覆盖和 daemon 可观测性已落地并通过真实 daemon 冒烟。剩余重点是连续多任务自动化、并发策略、IPC 真实闭环、decision retry 验收和智能效果基准。

## r6 真实复测证据

已执行：

- `go test ./internal/systemd ./internal/runtime ./internal/context ./internal/tools ./internal/agent`
- `go test ./...`
- `go build -o 5hagent cmd/5hagent/main.go`
- 在 `/Users/bytedance/Proj/5hWorkSpace` 执行 `5hagent daemon --poll 1s`

真实产物：

- `.5hagent/reports/agent-1.md`：daemon 启动 `agent-1`，完成 `systemd-multi-a`。
- `.5hagent/reports/agent-2.md`：同一 daemon 生命周期内继续启动 `agent-2`，完成 `systemd-multi-b`。
- `.5hagent/agents/agent-1/logs/*.md` / `.5hagent/agents/agent-2/logs/*.md`：记录 process worklog。
- `.5hagent/reports/systemd-r6-p0-smoke.agent-1.20260629-184516.038036000.md`：r6 P0 冒烟，report 包含 `task/task_title/source_event/worklog`。
- `.5hagent/task.md` 中 `systemd-r6-p0-smoke` 已由 daemon 自动更新为 `completed`。

当前边界：

- `AgentSystemd.RunProcess` 当前禁止已有 running process 时启动新进程，所以当前是串行多 AgentProcess，不是并发多 Agent。
- no-tool/text 基准已通过“未绑定工具模型 + ToolChoiceForbidden”双保险修复；后续不要退回纯 prompt 约束。

## r6 下一阶段 TODO

| 优先级 | 任务 | 验收标准 |
| --- | --- | --- |
| Done | daemon task 状态闭环 | AgentProcess 成功后源任务自动变为 `completed`；失败后变为 `failed`；不需要人工改 task 文件才能继续调度下一个任务 |
| Done | process report 绑定 task trace | report/worklog 至少包含 `task_id`、`task_title`、触发事件 ID、启动时间、结束时间；文件名不覆盖旧 report |
| Done | daemon 可观测性 | `5hagent daemon` 至少输出 process start/exited/failed、task id、report path；长期运行时用户能判断当前是否卡住 |
| Done | 连续多任务验收自动化 | `TestTaskFileEventSourceSkipsCompletedTask` 验证已 completed 的任务会被跳过，下一个 pending/in_progress 任务进入 `task.created` |
| Done | 明确并发多 Agent 策略 | 当前明确为串行多 AgentProcess；`TestRunProcessRejectsConcurrentProcess` 锁住已有 running process 时拒绝启动第二个进程 |
| Done | IPC 基础闭环测试 | `TestIPCMessageRoundTrip` / `TestIPCRejectsInvalidMessage` 覆盖 send/recv、mailbox 清空和非法消息拒绝 |
| Done | decision 失败重试验收 | `TestRunRetriesFailedTaskOnce` 覆盖 retry 上限；`TestParseDecisionRejectsInvalidJSON` 覆盖非法 action/spec/skill/unknown field |
| Done | 智能效果基准任务集 | r5 已跑纯文本、禁止工具、只读工具；`base.list_dir` 通过；no-tool/text 通过，worklog 无 Tool Event |
| Done | daemon 策略参数化 | `maxRetry`、`stalledAfter` 已通过 systemd options 和 CLI flags 注入；`maxConcurrency` 暂不暴露，当前策略明确串行 |
| Done | 清理测试 workspace 产物规范 | `scripts/archive_systemd_smoke.sh` 默认 dry-run，`--apply` 时按日期归档 systemd smoke report/worklog |

## r6 推荐实现顺序

1. 已完成 task trace：`AgentProcess.SourceTask` 记录 `task_id/task_title/event_id`，`ProcessSpec` 保持纯净。
2. 已完成状态闭环：`Runtime.RunProcess` 根据 `SourceTask` 更新 task 状态。
3. 已完成 report 命名：当前为 `<task-id>.<process-id>.<timestamp>.md`。
4. 已补连续多任务选择测试：completed 任务会被跳过，后续 pending/in_progress 任务会生成 `task.created`。
5. 下一步评估并发多 Agent；没有明确资源隔离前，先把当前串行策略写清楚。

## r6 本轮验证结果

- `go test ./internal/systemd ./internal/runtime ./internal/context ./internal/tools ./internal/agent` 通过。
- `go test ./...` 通过。
- `go build -o 5hagent cmd/5hagent/main.go` 通过。
- `git diff --check` 通过。
- `./5hagent daemon --help` 通过，包含 `--max-retry` 和 `--stalled-after`。
- `scripts/systemd_l4_audit.sh --mode no-tool ...` 通过，r5 no-tool/text worklog 无 Tool Event。
- `scripts/systemd_l4_audit.sh --mode require-tool ...` 通过，r2 list_dir worklog 有 Tool Event。
- 真实 daemon 冒烟通过：`systemd-r6-p0-smoke` 自动更新为 `completed`，report 写到 `.5hagent/reports/systemd-r6-p0-smoke.agent-1.20260629-184516.038036000.md`，report 包含 task trace 和 worklog path。
- P1 单测补齐：串行策略、IPC send/recv、completed task 跳过均有测试覆盖。
- decision 验收补齐：retry 上限和 strict JSON 校验均有测试覆盖。
- L4 智能效果 r2：`systemd-l4-listdir-r2` 正确触发 `base.list_dir`；`systemd-l4-notool-r2` 和 `systemd-l4-text-r2` 仍出现一次空参数 `base.read_file`，虽然最终文本正确，但违反 no-tool/text 基准。
- L4 智能效果 r5：重建二进制后复测 `systemd-l4-notool-r5` 和 `systemd-l4-text-r5`，两者 report 正确、task 自动 completed、worklog 无 Tool Event；no-tool/text 基准通过。

## r6 下一步

1. 已补 L4 基准自动判定脚本：`scripts/systemd_l4_audit.sh --mode no-tool|require-tool <worklog...>`。
2. 已完成 daemon 策略参数化：`--max-retry`、`--stalled-after` 接入 `systemd.WithMaxRetry/WithStalledAfter`。
3. 长期保留 provider/tool_choice 行为记录：no-tool 当前通过“未绑定工具模型 + ToolChoiceForbidden”双保险实现，不要退回纯 prompt 约束。
4. 剩余是否实现并发多 Agent 需要单独设计资源隔离；当前不作为默认 TODO 自动推进。

## r6 测试规范入口

详细测试矩阵写在 `doc/runtime/agent-systemd-test.md`。后续 Agent Systemd 改动必须先更新该文档里的“预期行为”和“验证命令”，再改代码。

---

# TODO.messages.md - Agent Systemd TODO 复审 (r5)

> 2026-06-29 复审当前工作树后更新。核对范围：`cmd/5hagent/main.go`、`internal/systemd/systemd.go`、`internal/runtime/runtime.go`、`internal/runtime/event_source_task.go`、`internal/context/ctx.go`、`internal/tools/ipc_tool.go`、`internal/ipctypes/`、`internal/systemd/systemd_test.go`、`internal/runtime/process_test.go`、`doc/runtime/agent-systemd.md`、`README.md`、`AGENTS.md`。
>
> 结论：r4 中多数高优项已经落地，旧文档中“零调用方、零测试、反向依赖、`seen` 泄漏、payload 静默丢字段”等判断已过期。下面是当前剩余 TODO；r4 原文仅作为历史归档保留，不要按旧执行顺序继续改。

## r5 当前结论

已落地项：

- `cmd/5hagent/main.go` 已接入 `daemon` 子命令，组合 `systemd.New`、`StartTimer`、`NewTaskFileEventSource`、`EmitCurrentTask`、`Run(ctx, rt, rt)`。
- IPC 协议已抽到 `internal/ipctypes.Message`，`context` / `tools` / `systemd` 不再通过 `systemd.IPCMessage` 互相耦合。
- `WithToolRuntime` 和 `WithSystemRuntime` 均为 merge 模式，调用顺序不再隐式依赖。
- `timer.tick` 不进入 `seen` 去重表；`process.exited/process.failed/process.stopped` 会清理 retry key。
- `task.created` 已改用 `TaskCreatedPayload{process_spec, task_id, task_title, file_event}`，`dispatch` 按事件类型严格解析 payload。
- `doc/runtime/agent-systemd.md` 已把旧伪代码改为当前 `Run/dispatch/Decision/applyDecision` 结构，并折叠了大段“已完成”清单。
- 已新增 `internal/systemd/systemd_test.go` 和 `internal/runtime/process_test.go`，覆盖基础调度闭环、异步启动失败、高风险事件、retry 上限和 `Runtime.RunProcess` prompt 注入。

## r5 剩余 TODO

| 优先级 | 任务 | 当前证据 | 建议 |
| --- | --- | --- | --- |
| P0 | 跑当前验证并记录真实结果 | r4 文档声称 build/vet/pass，但这是旧状态；当前工作树已有新增代码和测试 | 执行 `go test ./internal/systemd ./internal/runtime ./internal/context ./internal/tools ./internal/agent`、`go build -o 5hagent cmd/5hagent/main.go`、`git diff --check`；只把真实结果写回文档 |
| P1 | 明确 daemon 的任务状态闭环 | `RunProcess` 写 process report/worklog，但当前 `daemon` 路径没有像 `RunTaskOnce` 一样把 `.5hagent/task.md` 的任务标记为 completed/failed | 先记录为 Stage 7 TODO；实现前决定状态更新应归 `Runtime.RunProcess`、systemd 事件消费方，还是 task event source 外层适配器 |
| P1 | 补 task trace 到 process report | `TaskCreatedPayload` 保留 `task_id/task_title`，但 `dispatch` 当前只取 `ProcessSpec`，`AgentProcess` / `processReport` 没有持久化 task 来源 | 保持 `ProcessSpec` 纯净；可在后续加 `AgentProcess.Source Event` 或轻量 metadata，避免把 task 字段塞进 `ProcessSpec` |
| P2 | 扩充 systemd 合约测试 | 已有基础测试，但缺少 `task.created` strict schema、unknown field 拒绝、`timer.tick` 不写 `seen`、进程结束清 retry 的定向断言 | 在 `internal/systemd/systemd_test.go` 增加小粒度语义测试，避免依赖行号和输出格式 |
| P2 | 决定 IPC 活跃度语义 | `internal/systemd/systemd.go` 仍有 TODO：`Send` 同步更新收信方 `LastActiveAt` | 维持当前行为可用；后续如要审计 IPC，改为 `ipc.message` 事件链并补测试 |
| P3 | daemon 长生命周期策略参数化 | `maxRetry=1`、`stalledAfter=30m` 写死在 `New()` | 等 daemon 真实使用后再加 functional options 或 config，暂不提前抽象 |

## r5 推荐执行顺序

1. 先跑验证命令，更新本文和 `doc/runtime/agent-systemd.md` 中的验证状态。
2. 补 P2 的小粒度 systemd 测试，锁住 r4 已修的协议和内存增长问题。
3. 设计 daemon 的任务状态闭环，再决定是否改代码；不要把 `task_id` 直接塞进 `ProcessSpec`。
4. 保留 IPC `LastActiveAt` TODO，等需要 IPC 审计或真实多进程通信后再改事件链。

## r4 已解决项索引

以下是 r4 曾指出、当前已由代码或文档落地的事项，仅用于追溯：

- 反向依赖：IPC 协议类型迁到 `internal/ipctypes.Message`。
- 长期运行状态：`timer.tick` 不写入 `seen`，进程结束类事件清理 retry key。
- payload 合约：`task.created` 使用 `TaskCreatedPayload`，`process.start/manual.request` 使用纯 `ProcessSpec`，并统一严格解析。
- runtime 注入：`WithToolRuntime` 和 `WithSystemRuntime` 均为 merge 模式。
- 端到端入口：`5hagent daemon` 已接通最小运行期路径。
- 测试：已有 systemd 基础调度测试和 runtime fake LLM 适配测试。
- 文档：`doc/runtime/agent-systemd.md` 已更新真实函数名和当前完成审计。
