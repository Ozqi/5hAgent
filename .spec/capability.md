# 主要外接能力 Spec

> 由 Claude Fable 5 于 2026-08-24 阅读 `internal/tools/*.go`、`internal/commands/*.go`、`internal/mcp/*.go`、`internal/toolmeta`、`internal/toolevent` 后重构。
> 覆盖范围：LLM tools、slash command handler、tool event、MCP 适配和 planned dynamic tools。

## 职责边界

能力层负责“模型或用户命令能调用什么”。固定能力包括文件、grep/glob、shell、skill、context；MCP 和动态 terminal tools 是显式接入能力，不阻塞默认启动。

```mermaid
flowchart LR
  Runtime --> Registry[tools.Registry]
  Registry --> Base[base.*\nread/write/edit/glob/grep/list/exec]
  Registry --> Skill[skill.skill]
  Registry --> Ctx[context.context]
  Registry -. explicit .-> MCP[mcp.<server>.<tool>]
  Registry -. planned .-> Terminal[terminal.<name>.<action>]
  Commands[/slash commands/] --> SkillCmd[/skill]
  Commands --> MCPCmd[/mcp]
  Commands --> Compress[/compress]
```

工具图见：[`diagrams/walle-capabilities.mmd`](diagrams/walle-capabilities.mmd)。

## 关键文件

| 文件 | 责任 |
| --- | --- |
| `internal/tools/registry.go` | Runtime-scoped 工具注册表。 |
| `internal/tools/read_file.go`、`read_md.go` | 文件和 Markdown 读取/section 操作。 |
| `internal/tools/write_file.go`、`edit.go` | 写文件和精确替换。 |
| `internal/tools/glob.go`、`grep.go`、`list_dir.go`、`exec_shell.go` | 检索和 shell。 |
| `internal/tools/context_tool.go`、`skill_tool.go`、`mcp_tool.go` | context、skill、MCP 工具适配。 |
| `internal/commands/*.go` | 纯 slash command handler。 |
| `internal/mcp/*.go` | MCP 配置类型和 stdio JSON-RPC client。 |
| `internal/toolevent`、`internal/toolmeta` | 展示元数据和工具事件。 |

## Registry 契约

- `Init(skillMgr)` 每次清空并注册 base tools；`skillMgr != nil` 时追加 `skill.skill`。
- `SetWorkspaceRoot` 必须在 `Init` 前设置，供本地路径工具解析相对路径。
- `RegisterContextTool` 必须在 `ToolInfos` 和模型 `WithTools` 前调用。
- `ReplaceContextTool` 用于模型切换后替换压缩用 LLM，然后 Runtime 重新 `WithTools`。
- `RegisterMCPTools` 保留给显式 MCP 连接，工具名固定为 `mcp.<server>.<tool>`。

## Slash commands

| 命令 | 处理函数 | 副作用 |
| --- | --- | --- |
| `/skill list/get/reload` | `commands.HandleSkill` | reload 成功后整体替换 Skill snapshot。 |
| `/compress` | `commands.HandleCompress` | 写 compact archive，替换消息上下文。 |
| `/mcp list/add/remove/enable/disable` | `commands.HandleMCP` | 读写 `~/.walle/mcp.json`，不启动 MCP server。 |

`/provider`、`/model`、`/session`、`/stop`、`/detach` 属于 Entry/Runtime/DaemonSession，不在 `internal/commands` 扩散。

## MCP 边界

- 默认 Runtime 初始化不连接 MCP server。
- `NewStdioClient` 是可用但未接默认启动链路的显式能力。
- MCP schema 和调用结果来自远端；本地只做命名隔离、参数透传和错误包装。
- 不要因为 `RegisterMCPTools` 当前少引用就删除，它是 planned/optional 接入点。

## 不要做

- 不绕过 Registry 私自暴露工具给 LLM。
- 不在 commands 中启动 TUI、daemon、Runtime 或 LLM 主循环。
- 不恢复固定 TaskList、`task.task`、`/task` 或隐式任务真源。
- 不给 planned dynamic tools 新增持久化，除非先更新 `.TODO/10_动态终端工具.md`。

## 验收

- 改工具 schema：检查模型可见名称、参数必填项、错误提示是否能指导下一次调用。
- 改 slash command：检查本地 TUI 和 daemon session 两条分派路径。
- 改 MCP：确认默认启动不会等待外部进程。
