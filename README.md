# 5hAgent

5hAgent 是一个学习型 Go + Eino Agent runtime：通过自己做一个tiny版Claude，可以让自己对Agent的工作原理更加深刻。

用尽量少的代码保留 ReAct 循环、工具调用、任务文件、报告文件、MCP/Skill 扩展这些关键骨架。


## 已完成

- **ReAct 循环** — LLM 多轮生成、工具调用、观察结果回灌。
- **Tool use** — 基础文件工具、流式工具调用收集、工具执行与 LLM stream 重叠。
- **文件任务** — `.5hagent/task.md` 是任务真源，支持 pending/in_progress/completed/failed 等状态。
- **无头运行** — `5hagent run` 从任务文件取一个任务执行，并写 Markdown 报告。
- **MCP / Skill** — 可接外部 MCP 工具，也可通过 Skill 注入可复用工作流。

## 快速开始

1. 需要：Go 1.24.2 或更高版本，Node.js 18.x 或更高版本
2. 执行安装脚本：

```bash
# 在仓库内开发时，安装当前工作区代码
bash install.sh

# 远程安装 master 版本
curl -fsSL https://raw.githubusercontent.com/Ozqi/5hAgent/master/install.sh | bash
```

3. 配置 LLM：`~/.5hAgent/.env`，不同 provider 的配置可以同时保留，`LLM_PROVIDER` 决定当前使用谁：

```env
LLM_PROVIDER=claude

LLM_CLAUDE_API_KEY=your_api_key
LLM_CLAUDE_BASE_URL=https://api.anthropic.com
LLM_CLAUDE_MODEL=claude-sonnet-4-6
LLM_CLAUDE_MAX_TOKENS=4096
LLM_CLAUDE_THINKING_BUDGET_TOKENS=0

LLM_OPENAI_API_KEY=dummy
LLM_OPENAI_BASE_URL=http://localhost:11434/v1
LLM_OPENAI_MODEL=qwen3:14b
LLM_OPENAI_MAX_TOKENS=4096
LLM_OPENAI_THINKING_BUDGET_TOKENS=0

AGENT_NAME=5hAgent
AGENT_CONTEXT_AUTO_COMPRESS=true
```

### 使用本地 Ollama

如果想先用本地模型跑通 baseline，可安装 Ollama 并拉取一个模型：

```bash
ollama pull qwen3:14b
# macOS 桌面版通常会自动启动服务；纯 CLI 环境可手动执行：
ollama serve
```

5hAgent 只区分两种接口风格：`claude` 和 `openai`。Ollama 通过 OpenAI-compatible `/v1` 接口接入，切到本地 Ollama 时只改当前 provider：

```env
LLM_PROVIDER=openai
LLM_OPENAI_BASE_URL=http://localhost:11434/v1
LLM_OPENAI_MODEL=qwen3:14b
LLM_OPENAI_API_KEY=dummy
```

Ollama 当前使用哪个模型由 `LLM_OPENAI_MODEL` 决定。例如使用 HuggingFace GGUF：

```env
LLM_OPENAI_MODEL=hf.co/bartowski/Qwen_Qwen3.6-27B-GGUF:Q3_K_M
```

Ollama 本地模型不校验 API key，`dummy` 即可。建议先用 `5hagent run` 执行一个只读任务验证 chat、工具调用和报告落盘。

## 无头运行

先在项目目录准备任务文件：

```bash
mkdir -p .5hagent
cat > .5hagent/task.md <<'EOF'
# Shared Task List

<!-- 5hagent:tasks:start -->
## Shared Tasks

### baseline-demo | 整理 baseline
- status: pending
- description: 阅读项目并输出一份极简 baseline 整理建议。
- created_at: 2026-06-16T00:00:00Z
- updated_at: 2026-06-16T00:00:00Z

<!-- 5hagent:tasks:end -->
EOF
```

执行一个任务并写报告：

```bash
5hagent run
# 或指定任务
5hagent run --task baseline-demo
# 脚本场景只保留 report 路径和错误
5hagent run --quiet
```

无头模式默认会在终端输出正常工作日志，包括 assistant 流式文本、工具调用和工具结果摘要。每次运行还会按 Agent 名称写一份 Markdown 工作日志，便于多个 Agent 并行或轮流运行时分开追踪：

```text
.5hagent/agents/<agent-name>/logs/<timestamp>-<task-id>.md
```

`--quiet` 只压制终端工作日志，项目目录下的 Agent 工作日志仍会写入。`--debug` 仍用于写入更完整的 DEBUG 文件日志。

输出报告默认位于：

```text
.5hagent/reports/<task-id>.md
```

## 内置命令

| 命令        | 说明                 |
| ----------- | -------------------- |
| `/task`     | 创建、更新、归档任务 |
| `/skill`    | 查看已加载技能       |
| `/compress` | 压缩上下文           |

## 目录结构

```
internal/
├── runtime/        # 无头/TUI 共享运行时：初始化配置、Agent、工具、任务、报告
├── agent/          # Agent 核心：ReAct 循环、工具调度
├── llm/            # LLM 客户端
├── tools/          # 文件读写、搜索、执行、task/skill/context/sys/mcp 工具
├── task/           # 文件任务模型、Markdown 持久化、状态流转
├── skill/          # 技能快照加载
├── context/        # 上下文和 session 管理
└── cli/            # TUI/CLI 展示层
cmd/5hagent/       # 入口
doc/               # 详细文档
```

## 文档

- [Runtime 运行时](doc/runtime.md)
- [Agent 架构](doc/agent.md)
- [工具系统](doc/tools.md)
- [上下文管理](doc/context.md)
- [Skill 使用](doc/skill.md)
- [LLM 客户端](doc/llm.md)
- [LLM 调用流程](doc/llm-call-flow.md)
