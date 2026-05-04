# 5hAgent

基于 Go + Eino轻量级Coding Agent实现。
持续运行直到完成所有task.md中的"Task"。

## 已完成

- **ReAct 循环** — 思考、行动、观察的自动化执行
- **Tool_use** — 只读工具并行执；LLM流式输出的同时启动工具执行。
<!-- - **上下文压缩** — `/compress` 手动触发，释放 token 压力 -->
- **MCP 扩展** — 支持接入外部 Model Context Protocol 服务
- **Skill 系统** — 可复用的工作流模
- **"Task"管理** — 持续运行直到完成所有task.md中的"Task"。

## 快速开始

1. 需要：Go 1.24.2 或更高版本，Node.js 18.x 或更高版本
2. 执行安装脚本：

```bash
   curl -fsSL https://raw.githubusercontent.com/Ozqi/5hAgent/master/install.sh | bash
```

3. 配置API key：`~/.5hAgent/.env`:

```env
LLM_API_KEY=your_api_key
LLM_BASE_URL=https://api.anthropic.com
LLM_MODEL=claude-sonnet-4-6
AGENT_NAME=5hAgent
```

## 内置命令

| 命令        | 说明                 |
| ----------- | -------------------- |
| `/task`     | 创建、更新、归档任务 |
| `/skill`    | 管理技能模板         |
| `/compress` | 压缩上下文           |

## 目录结构

```
internal/
├── agent/          # Agent 核心：ReAct 循环、工具调度
├── llm/            # LLM 客户端
├── tools/          # 文件读写、搜索、执行
├── skill/          # 技能管理
├── context/        # 上下文管理
└── cli/            # CLI 交互
cmd/5hagent/       # 入口
doc/               # 详细文档
```

## 文档

- [Agent 架构](doc/agent.md)
- [工具系统](doc/tools.md)
- [Skill 使用](doc/skill.md)
- [CLI 命令](doc/cli.md)
