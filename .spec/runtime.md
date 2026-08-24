# Runtime Spec

## 职责

`internal/runtime` 是装配层。它组合配置、Prompt、LLM、Agent、Context、Tools、Skill、hooks、通用 process report/worklog 和 daemon interactive session。

Runtime 不内置 TaskList、任务命令、任务 watcher、任务状态或 task report。

## 覆盖文件

| 文件 | 职责 |
| --- | --- |
| `runtime.go` | Runtime 初始化、模型切换、通用 ProcessRunner、process report。 |
| `daemon_session.go` | daemon 长驻会话、slash command、事件回放。 |
| `provider.go` | provider/model 列表、OAuth 与切换辅助。 |
| `worklog.go` | 通用 process 工作日志。 |
| `hooks.go`、`tool_stats.go` | 工具事件 hook 与失败统计。 |

## Options 与字段

`Options` 包含 `Debug`、`SessionID`、`ContinueLast`、`ProjectDir`、模型覆盖与 `PromptBase`。不存在 TaskList 或 MemoryContext 选项。

`Runtime` 持有 `Agent`、`CtxManager`、`MessageCtx`、session/prompt/model/project 信息、私有 `ToolRegistry`、hooks 和模型切换锁。

## 核心接口

| 接口 | 行为 | 调用方 |
| --- | --- | --- |
| `New` | 创建 Runtime 并完成装配。 | daemon、嵌入调用方 |
| `Close` | 关闭 logger。 | CLI defer |
| `SwitchModel` | 重建模型、替换 ContextTool、重新绑定工具。 | TUI/daemon session |
| `RunProcess` | 执行通用 `systemd.AgentProcess`。 | `AgentSystemd` |
| `NewDaemonSession` | 创建可 attach 的长驻交互会话。 | `walle daemon` |
| `Providers` / `ProviderModels` | provider/model picker 数据。 | `/provider`、`/model` |
| `StartOpenAILogin` | 发起 ChatGPT OAuth。 | `/provider openai` |
| `RecordToolEvent` | 记录交互工具失败和 hooks。 | daemon session |

不存在 `RunTaskOnce`、`RunTasksUntilDone`、`TaskProcessSpec` 或 `EmitCurrentTask`。

## 初始化流程

1. 加载配置并初始化 logger。
2. 确定 workspace 与项目数据目录。
3. 创建 session-backed `context.Manager` 并打开 message context。
4. 创建 LLM，加载 system prompt。
5. 创建 Agent，注入 Context Manager、debug 信息和 context window。
6. `tools.NewRegistry().Init(skillMgr)` 注册 base/skill 工具。
7. 注册 `context.context`，再收集 `ToolInfos` 并调用 `WithTools`。
8. 加载 hooks，返回 Runtime。

## 运行模式

| 模式 | Context | 产物 | 说明 |
| --- | --- | --- | --- |
| interactive daemon | 持久化 session | socket events、session JSONL | 默认 TUI 连接的长驻会话。 |
| local TUI embed | 持久化 session | TUI 状态、session JSONL | 保留给直接嵌入场景。 |
| generic process | 独立 session-backed context | process report、worklog | `AgentSystemd` 的通用 runner。 |

## AgentProcess 流程

1. 校验 `AgentProcess` 和 `ProcessSpec`。
2. 创建独立内存 message context。
3. 把 `ProcessSpec.SystemPrompt` 写为 system message。
4. 创建 process worklog，回填 `WorkLogPath`。
5. 接管工具事件 sink，调用 `Agent.RunStream`。
6. 恢复旧 sink。
7. 写 `<process-id>.<timestamp>.md` report，回填 `ReportPath`。
8. 输出 process completed/failed。

流程不读取任务文件，不回写任务状态，不要求任务 ID 或标题。

## DaemonSession

- daemon 固定创建一个 `DaemonSession`，没有 interactive 模式开关。
- `Snapshot` 返回 `ProcessSnapshot{Name:"interactive"}`。
- 普通输入启动一轮 `Agent.RunStream`；slash command 在 session 内处理。
- 事件历史仅驻留内存，daemon 退出后丢失。

## 状态与持久化

| 状态 | 路径或字段 |
| --- | --- |
| Session | `~/.walle/sessions/*.jsonl` |
| Process report | `<project>/.walle/reports/<process-id>.<timestamp>.md` |
| Process worklog | `<project>/.walle/agents/<process-id>/logs/*.md` |
| Tool stats | 用户级与项目级 `tool-failures.jsonl` |
| Hooks | `<project>/.walle/hooks.json` |

## 不变量

- `RegisterContextTool` 必须发生在模型 `WithTools` 前。
- `SwitchModel` 必须替换 ContextTool 模型引用并重绑完整工具集合。
- hooks 失败不得阻塞 Agent 成功路径。
- 默认启动不连接 MCP stdio server。
- 任务管理由 Skill、MCP 或外置动态工具提供；不向 Runtime 增加固定 TaskList 接口。
