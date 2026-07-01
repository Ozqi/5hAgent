# Agent Systemd 测试规范

> 由 GPT-5.5 于 2026-06-30 阅读 `internal/systemd/systemd.go`、`internal/runtime/runtime.go`、`internal/runtime/event_source_task.go`、`cmd/5hagent/main.go`、`TODO.messages.md` 后生成。本文记录 Agent Systemd 的验收口径，目标是让后续 Agent 能按同一标准复测 daemon、AgentProcess、IPC、report 和智能效果。

## 摘要

Agent Systemd 当前是最小 daemon 调度骨架：监听 Project 下 `.5hagent/task.md`，把 `pending/in_progress` 任务转换为 `task.created`，再通过 `Runtime.RunProcess` 启动 AgentProcess。测试必须同时覆盖确定性代码行为和真实 LLM 行为；前者用 Go test/fake runner，后者只在 `/Users/bytedance/Proj/5hWorkSpace` 运行。

```mermaid
flowchart LR
    task[".5hagent/task.md"] --> source["TaskFileEventSource"]
    source --> sys["AgentSystemd.Run"]
    sys --> runner["Runtime.RunProcess"]
    runner --> agent["Agent.RunStream"]
    agent --> report["process report / worklog"]
```

## 测试分层

| 层级 | 目标 | 入口 | 是否依赖 LLM |
| --- | --- | --- | --- |
| L0 合约测试 | 锁住 systemd 状态机和 payload 规则 | `go test ./internal/systemd` | 否 |
| L1 runtime 适配 | 验证 ProcessSpec.SystemPrompt 注入和 context runtime | `go test ./internal/runtime` | 否 |
| L2 构建集成 | 确认 CLI 和包依赖可编译 | `go test ./...` / `go build` | 否 |
| L3 daemon 冒烟 | 真实 daemon 启动 AgentProcess | `/Users/bytedance/Proj/5hWorkSpace` | 是 |
| L4 智能效果 | 验证模型是否按任务、工具和退出条件行动 | 固定任务集 | 是 |

## 固定验证命令

在当前 5hAgent 仓库根目录执行：

```bash
go test ./internal/systemd ./internal/runtime ./internal/context ./internal/tools ./internal/agent
go test ./...
go build -o 5hagent cmd/5hagent/main.go
git diff --check
```

通过标准：

- 所有命令退出码为 0。
- `go test ./...` 不允许只跑局部后声称全量通过。
- 改 daemon wiring 时必须额外执行真实 daemon 冒烟。

## 测试 workspace

真实 LLM/headless/daemon 测试统一使用：

```text
/Users/bytedance/Proj/5hWorkSpace
```

禁止把真实测试散落到：

- `/private/tmp`
- 5hAgent 仓库内临时 `testspace`
- 其他未说明的项目目录

Project 数据目录：

```text
/Users/bytedance/Proj/5hWorkSpace/.5hagent/
```

需要检查的产物：

```text
.5hagent/task.md
.5hagent/reports/
.5hagent/agents/<agent-id>/logs/
~/.5hAgent/logs/
```

测试产物归档：

```bash
# dry-run
scripts/archive_systemd_smoke.sh
# 确认后移动到 .5hagent/archive/systemd-smoke/<timestamp>/
scripts/archive_systemd_smoke.sh --apply
```

归档脚本只移动 `systemd-*.md` report 和 `*systemd-*.md` worklog，不删除文件。

## L0 合约测试清单

`internal/systemd/systemd_test.go` 至少覆盖：

- `RunProcess` 成功后产生 `process.exited`。
- invalid `ProcessSpec` 会产生 `process.failed`。
- `task.created` 使用 `TaskCreatedPayload` 严格 schema，拒绝 unknown field。

测试原则：

- 优先 fake runner。
- 不断言 ANSI、padding、文件 mtime 这类易碎格式。
- 每个测试只锁一个语义，不写大而全的集成测试。

## L1 runtime 适配清单

`internal/runtime` 测试至少覆盖：

- `Runtime.RunProcess` 将 `ProcessSpec.SystemPrompt` 注入 process context。
- `WithSystemRuntime` 和 `WithToolRuntime` 可任意顺序合并，不覆盖彼此字段。
- `TaskProcessSpec` 不把 task id 塞进 `ProcessSpec`。
- `NewTaskFileEventSource` 只把 task 信息放进 `TaskCreatedPayload`。

## L3 daemon 冒烟

### 单任务

在 `.5hagent/task.md` 中准备：

```markdown
### systemd-daemon-smoke | Agent Systemd daemon 冒烟
- status: pending
- description: 请只回复一句中文：Agent Systemd daemon 已执行当前任务。不要调用工具，不要修改文件。
- created_at: 2026-06-30T00:00:00Z
- updated_at: 2026-06-30T00:00:00Z
```

启动：

```bash
cd /Users/bytedance/Proj/5hWorkSpace
/path/to/current/5hAgent/5hagent daemon --poll 1s
```

当前通过标准：

- 生成 `.5hagent/reports/<task-id>.<process-id>.<timestamp>.md`。
- 生成 `.5hagent/agents/<process-id>/logs/<timestamp>-<task-id>.md`。
- report 中 `status: completed`。
- report 中包含 `task`、`task_title`、`source_event`、`worklog`。
- task 状态自动从 `pending` 变为 `completed`。
- worklog 中有 assistant 最终输出。
- daemon stdout 输出 `process start` 和 `process completed/failed`，并带 task id / report path。

### 连续多任务

步骤：

1. 准备 `systemd-multi-a` 为 `pending`。
2. 启动 daemon，等待 `agent-1` report。
3. 等 A 自动变为 `completed`，新增 `systemd-multi-b` 为 `pending`。
4. 等待 `agent-2` report。
5. 中断 daemon。

当前通过标准：

- 同一 daemon 生命周期内出现 `agent-1` 和 `agent-2`。
- 两个 report 分别包含 A/B 的期望输出。
- A/B 两个 task 都自动变为 `completed`。
- `TestTaskFileEventSourceSkipsCompletedTask` 覆盖 completed 任务跳过逻辑，避免完全依赖人工 daemon 冒烟。

当前不应声称：

- 不应声称已支持并发多 Agent。
- `TestRunProcessRejectsConcurrentProcess` 已锁住当前串行策略；如果后续实现并发，必须先改该测试和本文并发验收标准。

## L4 智能效果任务集

每次切换 provider、prompt 或 tool 协议后，至少跑以下任务：

| ID | 任务 | 预期 |
| --- | --- | --- |
| `systemd-text-smoke` | 只回复固定中文，不调用工具 | 不应出现工具调用；输出应匹配任务 |
| `systemd-readonly-tool-smoke` | 必须调用 `base.list_dir` 列出 workspace | worklog 出现工具调用；报告描述真实条目 |
| `systemd-no-tool-smoke` | 明确禁止工具调用 | 不应调用工具；如调用需记录 provider/prompt 问题 |
| `systemd-ipc-smoke` | 两个 AgentProcess 通过 `sys.ipc` 传短消息 | 只传 `summary/artifact`，不共享 context |

智能效果判定：

- 完成任务不等于通过；必须检查是否违反“不要调用工具”等约束。
- 如果模型输出内部协议标记，如 `<tool_call>...</tool_call>`，记录为 provider/proxy 问题。
- 如果 report 正确但 task 状态未闭环，记录为 daemon 问题。

2026-06-30 r2 结果：

- `systemd-l4-listdir-r2` 通过：worklog 出现 `base.list_dir`，report 正确描述 workspace 顶层条目。
- `systemd-l4-notool-r2` 未通过：worklog 出现一次空参数 `base.read_file`，虽然最终文本正确。
- `systemd-l4-text-r2` 未通过：worklog 出现一次空参数 `base.read_file`，虽然最终文本正确。

判定：daemon 调度和 report/task 状态闭环可用；no-tool/text 的工具抑制仍需从 provider/tool_choice 或 Agent 工具绑定策略修正。

2026-06-30 r5 结果：

- `systemd-l4-notool-r5` 通过：worklog 无 `Tool Event`，report 输出指定文本。
- `systemd-l4-text-r5` 通过：worklog 无 `Tool Event`，report 输出指定文本。
- 修正方式：Agent Systemd 进程发现源 task 描述包含“不要调用工具/严禁调用任何工具”时，临时切到未绑定工具的模型，并同时传 `ToolChoiceForbidden`；进程结束后恢复原模型。

自动判定：

```bash
scripts/systemd_l4_audit.sh --mode no-tool <worklog.md>...
scripts/systemd_l4_audit.sh --mode require-tool <worklog.md>...
```

已验证：

- r5 no-tool/text worklog 在 `--mode no-tool` 下通过。
- r2 list_dir worklog 在 `--mode require-tool` 下通过。
- r2 no-tool worklog 在 `--mode no-tool` 下失败，能捕获历史违规样本。

## Report 验收标准

后续实现应让 process report 至少包含：

```text
process_id
task_id
task_title
source_event_id
status
started_at
ended_at
worklog_path
exit_condition
agent_output
error
```

文件命名不得覆盖历史结果。推荐：

```text
.5hagent/reports/<task-id>/<process-id>-<timestamp>.md
```

如果保持扁平目录，文件名至少包含 task id 和 timestamp：

```text
.5hagent/reports/<task-id>.<process-id>.<timestamp>.md
```

## Daemon 状态闭环验收

完成后必须满足：

- AgentProcess 成功退出后，源 task 自动更新为 `completed`。
- AgentProcess 失败后，源 task 自动更新为 `failed`。
- 状态更新时间写入 `updated_at`。
- 多个 pending task 会被依次消费，不需要人工改状态。
- process report 能反查源 task。
- 同一 task 不会因 file watcher 重复事件被重复启动。

## 并发多 Agent 验收

如果实现并发，必须先引入显式策略：

```text
max_concurrency
per_task_dedup
per_project_workspace
report_path_isolation
tool_event_sink_isolation
```

通过标准：

- 两个 pending task 可同时处于 running。
- 两个 AgentProcess worklog 不串线。
- 两个 AgentProcess report 不覆盖。
- IPC 可向指定 `process_id` 投递。
- 一个进程失败不影响另一个进程完成。

未实现前，文档和 CLI 只能声称“串行多 AgentProcess”。

## IPC 验收

IPC 测试要验证：

- `sys.ipc send` 自动填 `from = current process id`。
- `to` 必须是存在的 process id。
- `summary` 和 `artifact` 至少有一个非空。
- `recv` 拉取后 mailbox 清空。
- IPC 不读写对方 context。
- IPC 当前不维护活跃度语义；后续若改成 `ipc.message` 事件链，需要同步更新测试。

当前已有合约测试：

- `TestIPCMessageRoundTrip`：验证 send/recv 和 mailbox 清空。
- `TestIPCRejectsInvalidMessage`：验证缺少 target、target 不存在、空消息会被拒绝。

## 失败记录模板

每次真实 daemon 测试失败，都在 `TODO.messages.md` 追加：

```markdown
## YYYY-MM-DD Agent Systemd 复测

- 环境：provider/model/base_url
- 命令：...
- 任务 ID：...
- 预期：...
- 实际：...
- 证据：
  - report: ...
  - worklog: ...
  - log: ...
- 判断：代码问题 / provider 问题 / 测试任务问题 / 文档过期
- 下一步：...
```

## 完成审计

Agent Systemd 相关改动完成前，必须逐项回答：

- 是否更新了 `TODO.messages.md` 中对应任务状态？
- 是否更新了 `doc/agent-systemd.md` 的当前边界？
- 是否更新了本文的测试入口或验收标准？
- 是否跑过固定验证命令？
- 是否在 `/Users/bytedance/Proj/5hWorkSpace` 做过真实 daemon 测试？
- 是否检查 report/worklog/task 状态三者一致？
- 是否明确说明当前是串行还是并发多 Agent？
