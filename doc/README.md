# 5hAgent 代码结构

```
5hAgent/
├── cmd/5hagent/
│   └── main.go                 # 程序入口：初始化 Agent、TUI、工具注册，启动交互界面
│
├── internal/
│   ├── agent/                  # Agent 核心
│   │   ├── agent.go            # ReAct 循环、流式 LLM 调用、上下文初始化、skill 注入
│   │   └── tool_use.go        # ToolCall 解析、并发/串行执行、错误归类
│   │
│   ├── cli/                    # 终端 UI（TUI）
│   │   ├── tui.go             # Bubble Tea 主界面：对话面板 + 状态栏，/task /skill /compress 命令入口
│   │   ├── ui.go              # PrintError（stdout 退路），其余 Print* 函数已注释
│   │   └── markdown_stream.go  # Markdown 终端渲染（标题/代码块/列表/引用/内联格式）
│   │
│   ├── commands/               # Slash 命令处理
│   │   ├── skill.go           # /skill list/enable/disable
│   │   ├── task.go            # /task create/update/get/list/delete/archive/reopen
│   │   └── compress.go        # /compress（手动触发当前上下文压缩）
│   │
│   ├── context/                # 消息上下文管理
│   │   └── ctx.go             # Context 创建/克隆、消息存储、LLM 压缩（超 MaxMessages 后触发）
│   │
│   ├── llm/                    # LLM 客户端
│   │   └── client.go          # 从 .env 创建 Claude ChatModel（Eino 接口封装）
│   │
│   ├── logger/                 # 日志与输出
│   │   ├── logger.go          # DEBUG/INFO/WARN/ERROR 带标签日志
│   │   ├── color.go           # ANSI 颜色（Red/Green/Cyan/Gray/Bold...）
│   │   └── toolprint.go       # 工具调用的终端格式化输出（ToolCall/ToolResult/ToolError）
│   │
│   ├── mcp/                    # MCP 协议定义
│   │   └── mcp.go             # ServerConfig/ToolSpec，FullToolName，Client 接口
│   │
│   ├── skill/                  # 技能加载与管理
│   │   └── skill.go           # 从 .5hagent/skills/*/SKILL.md 加载，支持启用/禁用
│   │
│   ├── task/                   # 任务列表持久化
│   │   ├── tasklist.go        # Task CRUD、Markdown 读写、历史归档
│   │   └── task_actions.go    # TaskActionRequest 分发（create/update/get/list/delete/archive/reopen）
│   │
│   ├── toolmeta/               # 工具元数据注册
│   │   └── toolmeta.go        # 工具分类（base/task/skill/mcp）、只读属性、显示名
│   │
│   ├── tools/                  # 工具实现
│   │   ├── registry.go        # 工具注册表 InitRegistry / GetAllTools / RegisterMCPTools
│   │   ├── read_file.go       # 读文件（offset/limit 范围）
│   │   ├── write_file.go      # 写文件（自动创建父目录）
│   │   ├── edit.go            # 字符串替换编辑（old_string → new_string）
│   │   ├── exec_shell.go      # 执行 shell 命令，返回 stdout/stderr/returncode
│   │   ├── grep.go            # ripgrep 搜索（fallback grep），支持正则和文件类型过滤
│   │   ├── glob.go            # 文件模式匹配（* 和 **），支持递归
│   │   ├── list_dir.go        # 目录列表（递归/非递归），返回文件/目录/大小
│   │   ├── task_tool.go       # TaskList 的 Eino Tool 封装，供 Agent 调用
│   │   ├── skill_tool.go      # SkillManager 的 Eino Tool 封装，供 Agent 调用
│   │   ├── mcp_tool.go        # MCP 工具的 Eino Tool 封装
│   │   └── tools_test.go      # 工具测试
│   │
│   └── utils/                  # 公共工具函数
│       ├── utils.go           # Load(dir, name) 加载 prompt/*.md
│       ├── tokenBudget.go     # TokenBudget 累计 token 上限检查
│       └── utils_test.go
│
├── doc/                        # 文档（与代码目录一一对应）
│   ├── README.md              # 本文件
│   ├── agent.md               # Agent 主循环、ReAct、工具执行
│   ├── tools.md               # 工具注册、并发策略
│   ├── context.md             # 消息上下文、LLM 压缩
│   ├── skill_injection.md     # Skill 加载与注入
│   ├── prompt.md              # prompt 文件加载流程
│   ├── commonds.md            # /skill 和 /task 命令
│   ├── mcp.md                 # MCP 模块：Client 接口、工具注册、工具名格式
│   ├── llm.md
│   ├── logger.md
│   └── cli.md
│
└── prompt/                     # System prompt 文件
    └── main.md                # Agent 主 prompt
```

## 关键数据流

```
main.go
  → agent.NewAgent        # 创建 Agent（加载 skill、初始化 token budget）
  → tools.InitRegistry    # 注册 base/tools + task + skill + MCP 工具
  → cli.LaunchTUI         # 启动 TUI
      → agent.RunStream   # ReAct 循环（stream 流式 LLM）
          → tools.*       # 执行具体工具
          → context       # 管理消息历史，支持压缩
          → skill         # 注入已启用的 skill
```

## 关键类型

| 类型 | 文件 | 作用 |
|------|------|------|
| `agent.Agent` | `agent/agent.go` | ReAct 循环核心，协调 LLM/工具/上下文 |
| `TaskList` | `task/tasklist.go` | 任务列表（Markdown 持久化） |
| `Context` | `context/ctx.go` | 单次对话的消息历史 |
| `Manager` | `context/ctx.go` | 管理多个 Context，支持压缩 |
| `Skill.Manager` | `skill/skill.go` | 技能加载与启用状态 |
| `toolmeta.Meta` | `toolmeta/toolmeta.go` | 工具元数据（分类/只读/显示名） |

## 文档规则

- 文档必须和代码匹配，优先写当前实现，不把计划写成既成事实
- 先给结构图，再给关键文件和关键函数
- 如果引用代码，优先使用相对路径链接

**最后更新**: 2026-04-26
