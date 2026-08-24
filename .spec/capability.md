# Capability Spec

## 职责

能力层把模型或用户命令可调用的能力统一收口：本地 LLM tools、工具展示元数据、工具事件、MCP 适配和 slash commands。固定能力覆盖文件、shell、skill 和 context 操作。

任务管理不是固定 Runtime 能力；需要时由 Skill、MCP 或外置动态工具提供。

## 覆盖范围

| 路径 | 职责 |
| --- | --- |
| `internal/tools/registry.go` | 每个 Runtime 独立的工具注册表和元数据。 |
| `internal/tools/read_file.go`、`read_md.go` | 分段读取文件和 Markdown section 操作。 |
| `internal/tools/write_file.go`、`edit.go` | 创建、覆盖与精确替换。 |
| `internal/tools/glob.go`、`grep.go`、`list_dir.go` | 文件与文本检索。 |
| `internal/tools/exec_shell.go` | 在 workspace root 下执行 shell。 |
| `internal/tools/skill_tool.go` | 查询当前 skill snapshot。 |
| `internal/tools/context_tool.go` | 暴露 `context.context`。 |
| `internal/tools/mcp_tool.go`、`internal/mcp/*.go` | MCP 适配。 |
| `internal/toolmeta`、`internal/toolevent` | 工具元数据与事件。 |
| `internal/commands` | `/skill`、`/compress`、`/mcp` 的纯命令处理。 |

## Registry 接口

| 接口 | 行为 | 稳定约束 |
| --- | --- | --- |
| `NewRegistry()` | 创建空注册表。 | 初始化 `meta` map。 |
| `SetWorkspaceRoot(root)` | 设置相对路径解析根。 | 必须在 `Init` 前调用。 |
| `Init(skillMgr)` | 清空并注册固定本地工具。 | 每次调用重置工具和元数据。 |
| `All()` | 返回工具切片副本。 | 调用方不能修改内部切片。 |
| `ToolInfos(ctx)` | 收集模型 schema。 | 必须在 `WithTools` 前调用。 |
| `RegisterContextTool(llm,promptDir)` | 注册 `context.context`。 | 必须在模型绑定前执行。 |
| `ReplaceContextTool(llm,promptDir)` | 切模型后替换 context tool。 | 替换后需要重新 `WithTools`。 |
| `RegisterMCPTools(server,client,specs)` | 注册远端 MCP tools。 | 工具名用 `mcp.<server>.<tool>`。 |

## 默认 LLM 可见工具

| 工具 | 读写 | 行为 |
| --- | --- | --- |
| `base.read_file` | read-only | 按 offset/limit 返回带行号文本。 |
| `base.read_md` | read/write | 按标题读写 Markdown section。 |
| `base.write_file` | write | 创建父目录并覆盖写入。 |
| `base.edit` | write | 精确字符串替换。 |
| `base.glob` | read-only | 返回 glob 匹配路径。 |
| `base.grep` | read-only | 返回正则文本匹配。 |
| `base.list_dir` | read-only | 返回目录项。 |
| `base.exec_shell` | side-effect | 在 workspace root 运行 shell。 |
| `skill.skill` | read-only | 查询启动时 skill snapshot。 |
| `context.context` | write | inspect/pin/edit/audit/compress 当前上下文。 |

`mcp.<server>.<tool>` 只在显式连接并注册后可见。

## Slash commands

| 命令 | 处理函数 | 副作用 |
| --- | --- | --- |
| `/skill list/get/reload` | `commands.HandleSkill` | reload 整体替换 Manager snapshot。 |
| `/compress` | `commands.HandleCompress` | 写 compact archive，替换消息上下文。 |
| `/mcp list/add/remove/enable/disable` | `commands.HandleMCP` | 写 `~/.walle/mcp.json`。 |

`/provider`、`/model`、`/session`、`/stop`、`/detach` 由 TUI 或 daemon session 处理。不存在固定 `/task` 或 `/run`。

## MCP 与动态能力边界

- 默认 Runtime 初始化不等待 MCP server。
- MCP 工具完整名固定为 `mcp.<server>.<tool>`。
- MCP schema 和调用结果来自远端；本地只做命名隔离、参数透传和错误包装。
- 任务管理可经 Skill 指导 Agent，或由 MCP/外置动态工具提供；本次没有新增接口。

## ToolEvent

工具执行 -> Agent `toolEventSink` -> Runtime/Daemon/TUI/worklog/logger。`toolmeta` 和 `toolevent` 不执行工具，也不保存业务状态。

## 禁止

- 工具绕过 Registry 私自暴露给 LLM。
- 默认启动时阻塞等待 MCP server。
- 在 commands 中启动 TUI、daemon、Runtime 或 LLM 主循环。
- 为已删除的 TaskList 恢复固定 `task.task`、`/task` 或隐式任务真源。
