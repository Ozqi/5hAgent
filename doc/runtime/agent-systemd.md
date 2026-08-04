# 5hAgent Daemon 重做设计

> 由 GPT-5.5 于 2026-08-04 基于当前 `cmd/5hagent`、`internal/runtime`、`internal/systemd`、`internal/tui` 文档和问题复盘更新。
> 本文是 daemon/supervisor 重做设计。当前代码已先落一层过渡：固定 `supervisor.sock`，删除 `daemon-*.sock` 聚合；完整 `5hagentd` 二进制拆分尚未完成。

## 结论

`daemon`、`supervisor` 和 `process` 不是一回事。下一版必须拆成两类程序语义：

| 程序 | 职责 | 数量 |
| --- | --- | --- |
| `5hagentd` | 用户级唯一 supervisor daemon，维护进程表、socket、生命周期、路由和恢复 | 一个 |
| `5hagent` | CLI/TUI client，只负责命令、渲染和连接 daemon | 多个短生命周期客户端 |

runtime Agent 实例不是 daemon。它是被 `5hagentd` 管理的 worker/process，可以有多个，对应 workspace、session、task 或交互会话。

## 设计边界

- `5hagentd` 是唯一监管进程；不允许每个 workspace 启一个 daemon。
- `5hagent` 不直接持有长期 runtime；默认连接 `5hagentd`。
- TUI 是客户端，attach 到 daemon 管理的某个 runtime Agent 实例。
- `ps` 列 runtime Agent 实例，不列一堆 daemon。
- `attach` 连接现有 runtime Agent 实例；没有目标时由 daemon 创建或让用户选择。
- `run` 可以作为一次性客户端请求 daemon 创建 task runner，也可以保留无 daemon fallback；但长期方向仍归 daemon 管。
- 当前 `internal/systemd` 中“AgentSystemd 同时像 daemon 又像 process table”的模型需要回退重做。

## 目标结构

```text
cmd/5hagentd
  -> internal/supervisor
     -> process table
     -> unix socket server
     -> workspace/session/task routing
     -> runtime agent child lifecycle

cmd/5hagent
  -> ps / attach / run / TUI
  -> unix socket client
  -> no long-lived runtime by default

runtime agent instance
  -> internal/runtime
  -> internal/agent
  -> tools / context / llm
```

是否物理拆成两个二进制：推荐拆。

- `5hagentd`：唯一 daemon/supervisor。
- `5hagent`：CLI/TUI client。

如果为了过渡期少改代码，可以暂时保留一个仓库、两个 `cmd/` 入口；不要再用一个 `5hagent daemon` 概念混掉 supervisor 和 runtime Agent。

## 当前过渡实现

这一轮先清理最容易制造脏状态的多 daemon/socket 路径：

- control socket 固定为 `~/.5hAgent/run/supervisor.sock`。
- `5hagent ps` 只查询这个固定 socket，不再扫描 `daemon-*.sock`。
- `attach` 直接使用 runtime Agent ID，例如 `interactive`，不再使用 `daemon-<pid>/interactive`。
- 默认 `5hagent` 启动前先尝试 attach `interactive`；只有连不上才启动后台交互 agent。
- 新 supervisor 启动时清理旧 `daemon-*.sock` 文件。

仍未完成：`5hagentd` 独立二进制、真正的 supervisor process table、多 workspace/runtime agent 生命周期管理。

## 核心流程

### 启动

```text
5hagent
  -> connect ~/.5hAgent/run/supervisor.sock
  -> missing: start 5hagentd once
  -> retry connect
  -> request default interactive agent for current workspace
  -> launch TUI client attached to that agent
```

### daemon

```text
5hagentd
  -> bind fixed user socket
  -> load process table metadata
  -> accept client requests
  -> create / stop / list / attach runtime Agent instances
```

### TUI attach

```text
TUI
  -> client sends attach(agent_id)
  -> daemon streams Agent events as NDJSON
  -> TUI sends user input / slash commands back to daemon
  -> daemon routes to target runtime Agent instance
```

## Process 模型

`Process` 只表示被 supervisor 管理的 runtime Agent 实例：

| 字段 | 含义 |
| --- | --- |
| `agent_id` | daemon 内唯一 ID |
| `workspace` | 项目目录 |
| `session_id` | runtime session |
| `kind` | `interactive` / `task` / `headless` |
| `state` | `starting` / `idle` / `busy` / `exited` / `failed` |
| `created_at` | 创建时间 |
| `updated_at` | 最近活动时间 |

daemon 自己不出现在 `ps` 的主列表里；最多在 `5hagent daemon status` 或 debug 输出里显示 supervisor 状态。

## Socket

固定用户级 socket：

```text
~/.5hAgent/run/supervisor.sock
```

不要再使用多个 `daemon-*.sock` 表示多个 daemon。多实例应该体现在 process table，而不是多个 supervisor。

## 废弃旧设计

以下旧设计要回退：

- `5hagent daemon --interactive` 为每个 workspace 起一个长期 daemon。
- `daemon-<pid>/interactive` 这种把 daemon 和 interactive process 绑死的 ID。
- `5hagent ps` 聚合多个 daemon socket。
- 安装脚本只靠杀旧 daemon 来修补多 daemon 残留。
- 文档中把 daemon、AgentProcess、runtime Agent instance 混称为 process。

## 实施原则

- 简洁实现不是偷懒不实施设计；必须把 supervisor、client、runtime Agent 三个边界真的切开。
- 代码上保持简洁：少抽象、少 helper、少兼容壳，不做通用分布式进程管理框架。
- 先做本机用户级单 supervisor，不做多用户、多机器、多权限系统。
- 先用 Unix Socket 和 NDJSON；协议字段少而稳定。
- 先支持 `ps`、`attach`、默认 TUI、创建/停止 interactive Agent；task runner 后续接入同一 process table。

## 最小落地顺序

1. 固定 `supervisor.sock`，删除 `daemon-*.sock` 聚合语义。
2. 把默认 `5hagent` 改成 client：先连接 `interactive`，缺失时再启动一个过渡 supervisor。
3. 新增 `cmd/5hagentd`，只负责固定 socket 和 process table。
4. 把 runtime Agent 实例从 daemon 概念里拆出来，作为 supervisor 管理对象。
5. 更新安装脚本：安装 `5hagent` 和 `5hagentd`，重启唯一 supervisor。
