# walle 架构拓扑

> 由 Claude Fable 5 于 2026-08-25 根据 `.spec/diagrams/walle-architecture-topology.json` 与当前模块文档收敛。
> 覆盖范围：默认入口、daemon attach、Runtime 装配、Agent 执行、持久化边界。

本页是面向读者的拓扑导读，不再维护第二份 JSON/Mermaid 事实源。架构图事实源集中在 [`.spec/diagrams/walle-architecture-topology.json`](../.spec/diagrams/walle-architecture-topology.json)，图源集中在 [`.spec/diagrams/`](../.spec/diagrams/)。

## 主链路

```mermaid
flowchart TB
  CLI[cmd/walle] -->|启动或复用| Daemon[walle daemon]
  CLI -->|LaunchAttachedTUI| TUI[internal/tui]
  TUI <-->|NDJSON attach/input/stop| Control[internal/systemd supervisor.sock]
  Daemon --> Runtime[internal/runtime]
  Daemon --> Session[DaemonSession]
  Control -->|interactive route| Session
  Session --> Runtime
  Runtime --> Agent[internal/agent]
  Runtime --> Context[internal/context]
  Runtime --> Registry[tools.Registry]
  Agent --> Registry
  Context --> Store[~/.walle/sessions/*.jsonl]
```

## 当前事实

1. 默认 `walle` 先 attach `~/.walle/run/supervisor.sock` 的 `interactive`；不存在时启动当前二进制的 `daemon` 子命令并重试。
2. `walle daemon` 固定创建 Runtime、DaemonSession、AgentSystemd 和 control server。
3. TUI 是 client：只处理输入、渲染、远端事件和本地 `/detach`、`/stop`。
4. DaemonSession 是 interactive adapter：处理 slash command、事件历史、订阅者和当前 run cancel。
5. Runtime 是装配层和模型状态真源：持有 Context、LLM、Agent、Tools、Skill 和通用 process runner。
6. Agent 只执行 ReAct：读写 Context、调用绑定模型、执行工具、写回 assistant/tool result。

## 持久化边界

| 路径 | 内容 |
| --- | --- |
| `~/.walle/.env` / `~/.walle/settings.json` | 用户配置 |
| `~/.walle/prompt/*.md` | prompt |
| `~/.walle/sessions/*.jsonl` | message session |
| `~/.walle/run/supervisor.sock` | daemon 控制 socket |
| `<project>/.walle/reports/*.md` | 通用 process report |
| `<project>/.walle/agents/*/logs/*.md` | 通用 process worklog |

## 不再维护的重复源

`doc/topology.json` 和 `doc/topology.mmd` 已删除。后续更新拓扑时：

1. 先改代码。
2. 再更新 `.spec/diagrams/walle-architecture-topology.json` 和相关 Mermaid 图。
3. 最后只在本页同步读者需要的摘要。
