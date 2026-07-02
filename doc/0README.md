# Stage 4 导读

这一阶段对应 `learn/stage-4-mcp-session-tui`。

如果说 Stage 3 开始整理“能力怎么组织”，那 Stage 4 更像是在回答“一个真实可用的 Agent 产品，还缺哪些使用层和扩展层能力”。所以这一阶段不只是继续加功能，而是开始把项目往“更完整的使用体验”推进。

## 这一阶段重点加入了什么

- 更明确的 TUI 交互体验
- Session 持久化，让对话不只是一次性过程
- MCP 相关基础设施，开始准备接入外部工具服务器
- 更清晰的命令入口，比如 `/skill`、`/task`、`/mcp`
- 更完整的模块化文档结构

可以先这样理解这些新概念：

- `TUI`：终端里的交互界面，不是图形界面，但比纯命令行更适合持续对话
- `Session`：一次对话会话，可以理解成“这一轮聊天和操作的记录”
- `持久化`：把原本只在内存里的内容保存到磁盘，下次还能继续用
- `MCP`：一种给模型接外部工具或服务的协议，可以先理解成“标准化的外部工具接入口”
- `Slash Command`：像 `/task`、`/skill` 这样的人类显式命令，不是模型自己决定调用的工具

## 推荐阅读顺序

这一阶段建议不要从庞大的架构图开始读，而是从“用户会感受到什么变化”开始：

1. [doc/cli.md](./cli.md)
   先看终端交互层发生了什么变化，理解为什么项目开始重视 TUI。
2. [doc/task.md](./task.md)
   再看任务系统是怎么和交互体验结合起来的。
3. [doc/skill.md](./skill.md)
   补足技能在这一阶段的实际使用方式。
4. [doc/mcp.md](./mcp.md)
   了解这一阶段为什么会开始引入 MCP，以及它目前处在什么状态。
5. [doc/agent.md](./agent.md) 和 [doc/context.md](./context.md)
   最后再回到底层，理解这些使用层变化是怎么落到主循环和上下文里的。

## 这一阶段最重要的理解点

建议重点理解下面三件事：

1. Agent 项目走到这里，已经不只是“能不能调用工具”，而是“人怎么持续使用它”。
2. 一旦开始有会话、命令、任务、技能这些层，项目就必须更在意边界是否清楚。
3. MCP 的意义不在于这一阶段已经完全成熟，而在于项目开始为外部扩展留出标准接口。

这三点比背所有模块名更重要。

## 读代码时建议关注什么

这一阶段建议优先看这些位置：

- `internal/cli/`
- `internal/context/session.go`
- `internal/commands/`
- `internal/mcp/`
- `internal/agent/tool_use.go`

带着下面几个问题去读会更清楚：

- 为什么到了这一阶段，TUI 会变得重要？
- 为什么对话记录要开始落盘，而不是一直只放在内存里？
- `/task` 这类命令和模型调用工具，有什么边界区别？
- MCP 现在是已经完整接入，还是还在搭基础设施？

## 和前几个阶段相比，这一阶段的变化本质是什么

Stage 1 到 Stage 3 更偏“让 Agent 本身成立”。Stage 4 开始更偏“让这个 Agent 真正变成一个长期可使用的系统”。这意味着项目关注点已经从单次执行，逐渐转向持续使用、信息保留和外部扩展。

## 这一阶段暂时还不必死抠什么

你第一次读到这里时，不需要一上来就完全理解：

- 全部 TUI 渲染细节
- 全部 MCP 协议细节
- 所有命令的完整参数设计
- 每个模块之间的完整调用链

先把“为什么会引入这些层”搞清楚，比一次性吃透实现更重要。

## 进入下一阶段前，你应该已经理解

- 为什么一个对外可用的 Agent 需要会话、命令和更完整的交互层
- 为什么 MCP 会成为后续扩展的重要方向
- 为什么项目在这个阶段开始更像“产品雏形”，而不只是实验性代码

理解这些之后，再切到 `learn/stage-5-current`，去看这一套基础能力是怎样收敛成当时的公开 baseline 的。

| 类型 | 文件 | 作用 |
|------|------|------|
| `Agent` | `agent/agent.go` | ReAct 循环核心，协调 LLM/工具/上下文 |
| `TaskList` | `task/tasklist.go` | 任务列表（Markdown 持久化） |
| `Context` | `context/ctx.go` | 单次对话的消息历史 |
| `Manager` | `context/ctx.go` | 管理多个 Context，支持压缩 |
| `Skill.Manager` | `skill/skill.go` | 技能加载与启用状态 |
| `toolmeta.Meta` | `toolmeta/toolmeta.go` | 工具元数据（分类/只读/显示名） |
| `AppModel` | `cli/tui.go` | TUI 主界面状态管理 |
| `LLMClient` | `llm/client.go` | LLM 模型客户端封装 |

## 快速开始

```bash
# 构建
go build -o 5hagent cmd/5hagent/main.go

# 运行（需要配置 .env）
./5hagent

# Debug 模式
./5hagent --debug
```

## 内置命令

| 命令 | 说明 |
|------|------|
| `/skill list/enable/disable <name>` | 管理技能 |
| `/task create/update/get/list/delete <id>` | 管理任务 |
| `/compress` | 手动压缩上下文 |
| `/mcp list/add/remove <name>` | 管理 MCP 服务器 |

## 文档规则

- 文档必须和代码匹配，优先写当前实现
- 先给结构图，再给关键文件和关键函数
- 如果引用代码，优先使用相对路径链接

**最后更新**: 2026-04-30
