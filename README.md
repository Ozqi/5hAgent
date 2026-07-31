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

3. 配置 LLM：`~/.5hAgent/.env`，当前模型使用 `provider/model` 格式；第一段选择 provider 配置块，后面的部分原样作为模型名传给上游 API。Provider 详情按 `LLM_<PROVIDER>_*` 保存：

```env
LLM_MODEL=mira/claude-opus-4-6

LLM_MIRA_FORMAT=claude
LLM_MIRA_API_KEY=local
LLM_MIRA_BASE_URL=http://127.0.0.1:8787
LLM_MIRA_MAX_TOKENS=4096
LLM_MIRA_THINKING_BUDGET_TOKENS=0
LLM_MIRA_STREAM=true

AGENT_NAME=5hAgent
AGENT_CONTEXT_AUTO_COMPRESS=true
```

这里的 `FORMAT=openai|claude` 表示接口协议，不是 provider 名。临时切换时可以不改 `.env`：

安装脚本遇到旧版 `LLM_PROVIDER` / `LLM_SUPPLIER` 配置时会先备份原文件；如果能找到对应 `LLM_<PROVIDER>_MODEL`，会补充 `LLM_MODEL=provider/model`，但不会删除旧变量。

```bash
5hagent --model openrouter/openrouter/owl-alpha run
5hagent --llm-model claude-opus-4-6 run
```

TUI 内可用 `/model provider/model` 临时切换当前 runtime 模型，不改写 `~/.5hAgent/.env`：

```text
/model
/model mira/gpt-5.5
```

### 使用本地 Ollama

如果想先用本地模型跑通 baseline，可安装 Ollama 并拉取一个模型：

```bash
ollama pull qwen3:14b
# macOS 桌面版通常会自动启动服务；纯 CLI 环境可手动执行：
ollama serve
```

5hAgent 只区分两种接口格式：`claude` 和 `openai`。Ollama 通过 OpenAI-compatible `/v1` 接口接入，可以保存成 `ollama` 供应商配置：

```env
LLM_MODEL=ollama/qwen3:14b
LLM_OLLAMA_FORMAT=openai
LLM_OLLAMA_BASE_URL=http://localhost:11434/v1
LLM_OLLAMA_API_KEY=dummy
```

Ollama 当前使用哪个模型由 `LLM_MODEL` 的后半段决定。例如使用 HuggingFace GGUF：

```env
LLM_MODEL=ollama/hf.co/bartowski/Qwen_Qwen3.6-27B-GGUF:Q3_K_M
```

Ornith-1.0 是面向 agentic coding 的开源模型，Ollama library 已提供 `ornith:9b` 和 `ornith:35b`。如果只想快速验证本地 Agent 链路，优先从 `ornith:9b` 开始；如果机器内存足够，可以再切到 `ornith:35b`：

```bash
ollama pull ornith:9b
5hagent --model ollama/ornith:9b run
```

如果 `ollama pull ornith:9b` 返回 `requires a newer version of Ollama`，先升级 Ollama 客户端再重试。

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

在 TUI 里可以显式输入 `/run`，让当前 runtime 连续执行 `.5hagent/task.md` 中的 `in_progress` / `pending` 任务，直到没有可运行任务为止。每个任务仍会写入 `.5hagent/reports/` 和 `.5hagent/agents/<agent>/logs/`。

实验性的 Agent Systemd 入口可以持续监听 `.5hagent/task.md`，把当前或变更后的 `pending/in_progress` 任务作为 AgentProcess 执行：

```bash
5hagent daemon
# 调整 task.md 轮询间隔
5hagent daemon --poll 2s
```

daemon 会输出 process start/completed/failed、task id 和 report path。AgentProcess 成功退出后，源任务会自动标记为 `completed`；失败时标记为 `failed`。进程报告使用 task/process/timestamp 命名，避免覆盖旧报告：

```text
.5hagent/reports/<task-id>.<process-id>.<timestamp>.md
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

- [Runtime 运行时](doc/runtime/runtime.md)
- [Agent 架构](doc/core/agent.md)
- [工具系统](doc/integrations/tools.md)
- [上下文管理](doc/core/context.md)
- [Skill 使用](doc/core/skill.md)
- [LLM 客户端](doc/config/llm.md)
- [LLM 调用流程](doc/config/llm-call-flow.md)
