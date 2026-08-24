# walle 拓扑来源说明

> 由 Claude Fable 5 于 2026-08-23 阅读 `cmd/walle/*.go`、`internal/runtime/{runtime,daemon_session}.go`、`internal/systemd/{systemd,control}.go`、`internal/tools/registry.go` 与 `internal/tui/{app,commands}.go` 后更新。
> 覆盖范围：默认入口、交互 daemon、control socket、Runtime 装配、通用 process 和工具边界。

本页是 [topology.json](topology.json) 的人工来源文档；[topology.mmd](topology.mmd) 只从 JSON 表达同一组关系。

## 当前主链路

### 默认 TUI

1. `walle` 进入 `runTUI`。
2. `startInteractiveClient` 尝试 attach `~/.walle/run/supervisor.sock` 中的 `interactive`。
3. 不存在时启动当前二进制的 `daemon` 子命令并重试。
4. `LaunchAttachedTUI` 接收历史与实时 `ProcessEvent`。
5. 普通输入由 `DaemonSession.Submit` 交给 `Agent.RunStream`。

### Daemon

1. `walle daemon` 以 `PromptBase=tui` 创建 Runtime。
2. 创建 `AgentSystemd`、`DaemonSession` 和 control server。
3. 固定提供一个 attachable interactive Agent。
4. 命令没有 `--poll`、`--interactive` 分支，也不监听任务文件。

### Runtime

1. 加载配置、日志、session 和 prompt。
2. 创建 LLM 与 Agent。
3. `Registry.Init(skillMgr)` 注册 `base.*` 和 `skill.skill`。
4. 注册 `context.context`，再把工具 schema 绑定给模型。
5. Runtime 可作为 `ProcessRunner` 执行通用 `AgentProcess`。

## 模块证据

| 模块 | 入口 | 当前职责 |
| --- | --- | --- |
| CLI | `cmd/walle/main.go` | 默认 TUI、daemon、ps、attach |
| 默认 attach | `cmd/walle/interactive_command.go` | 复用或启动 daemon |
| TUI | `internal/tui` | 输入、渲染、远端事件 |
| Runtime | `internal/runtime/runtime.go` | Agent/Context/LLM/Tools 装配与通用 process runner |
| Daemon session | `internal/runtime/daemon_session.go` | interactive 会话、slash command、事件回放 |
| Systemd | `internal/systemd` | 通用 `process.start` 调度与 Unix Socket 控制面 |
| Tools | `internal/tools/registry.go` | base/skill/context 固定工具，MCP 显式注册 |
| Session | `internal/context` | 消息与 JSONL 持久化 |

## 任务边界

Runtime 只提供通用 process 执行能力。启动事件为 `process.start`，payload 为 `ProcessStartPayload{process_spec}`；`ProcessSnapshot` 使用 `Name`。任务管理由 Skill、MCP 或外置动态工具提供。

## 持久化边界

| 路径 | 内容 |
| --- | --- |
| `~/.walle/.env` / `settings.json` | 用户配置 |
| `~/.walle/prompt/*.md` | prompt |
| `~/.walle/sessions/*.jsonl` | message session |
| `~/.walle/run/supervisor.sock` | daemon 控制 socket |
| `<project>/.walle/reports/*.md` | 通用 process report |
| `<project>/.walle/agents/*/logs/*.md` | 通用 process worklog |

## 同步规则

1. 代码是真源。
2. 先更新 `topology.json` 的节点、边与边界事实。
3. 再同步 `topology.mmd` 和本页。
4. Mermaid 不新增 JSON 中不存在的架构关系。
