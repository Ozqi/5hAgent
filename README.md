# 5hAgent

5hAgent 是一个用 Go 和 Eino 实现的轻量 Agent runtime。核心 runtime 排除 TUI、测试、注释和空行后不到一万行 Go 代码，适合直接阅读、调试和改造。

这套小体量实现完整串起了 ReAct 循环、流式工具调用、上下文与会话、文件任务、Skill 和 MCP 扩展边界，并同时支持交互式 TUI、无头任务和 daemon。Go 带来了单二进制部署、较少的运行依赖，以及适合流式处理和并发控制的运行时基础。

## 功能

- ReAct 多轮执行：模型生成、工具调用、结果回灌
- 内置文件、搜索、Shell、任务和上下文工具
- TUI 会话与可分离的后台 Agent
- 基于 `.5hagent/task.md` 的无头任务和 Markdown 报告
- 用户级与项目级 Skill
- MCP 配置管理
- OpenAI-compatible、Claude 和本地 Ollama 接口

## 安装

需要 Go 1.24.2 或更高版本。Node.js 只在使用部分 MCP server 时需要。

在仓库内安装当前代码：

```bash
bash install.sh
```

安装 `master` 版本：

```bash
curl -fsSL https://raw.githubusercontent.com/Ozqi/5hAgent/master/install.sh | bash
```

脚本会将二进制安装到 `~/.local/bin/5hagent`，并在首次安装时创建 `~/.5hAgent/.env`。如果命令不可用，请将安装目录加入 `PATH`：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## 配置

模型使用 `provider/model` 格式。`provider` 对应一组 `LLM_<PROVIDER>_*` 配置，`model` 原样传给上游接口。

编辑 `~/.5hAgent/.env`：

```env
LLM_MODEL=mira/claude-opus-4-6

LLM_MIRA_FORMAT=claude
LLM_MIRA_API_KEY=local
LLM_MIRA_BASE_URL=http://127.0.0.1:8787
LLM_MIRA_MAX_TOKENS=4096
LLM_MIRA_STREAM=true

AGENT_NAME=5hAgent
AGENT_CONTEXT_AUTO_COMPRESS=true
```

`FORMAT` 表示接口协议，目前支持 `openai` 和 `claude`。完整配置项见 [.env.example](.env.example) 和 [LLM 配置](doc/config/llm.md)。

也可以只为本次运行切换模型：

```bash
5hagent --model openrouter/openrouter/owl-alpha
5hagent --model openrouter/openrouter/owl-alpha run
```

### 本地 Ollama

先拉取并启动模型：

```bash
ollama pull qwen3:14b
ollama serve
```

然后在 `~/.5hAgent/.env` 中配置：

```env
LLM_MODEL=ollama/qwen3:14b
LLM_OLLAMA_FORMAT=openai
LLM_OLLAMA_BASE_URL=http://localhost:11434/v1
LLM_OLLAMA_API_KEY=dummy
```

Ollama 通过 OpenAI-compatible `/v1` 接口接入。本地服务不校验 API key 时可使用 `dummy`。

### ChatGPT OAuth

在 TUI 中输入 `/provider openai`，按提示完成浏览器登录，再用 `/model` 选择当前账号可用的模型。OAuth 凭据保存在 `~/.5hAgent/auth/codex.json`，不会写入项目目录、session 或 report。

## 使用

### 交互模式

在项目目录执行：

```bash
5hagent
```

默认入口会连接当前 workspace 的交互 Agent；不存在时自动在后台启动。退出 TUI 后，Agent 可以继续运行。

```bash
5hagent ps
5hagent attach interactive
```

`ps` 列出当前可连接的 Agent，`attach` 重新进入指定实例。`attach` 的进程 ID 支持 zsh Tab 补全。

### 文件任务

任务保存在项目目录的 `.5hagent/task.md`。可以在 TUI 中创建任务：

```text
/task create baseline-demo 整理项目 阅读项目并输出一份整理建议
```

无头模式每次执行一个 `in_progress` 或 `pending` 任务：

```bash
5hagent run
5hagent run --task baseline-demo
5hagent run --quiet
```

报告和工作日志分别写入：

```text
.5hagent/reports/<task-id>.md
.5hagent/agents/<agent-name>/logs/<timestamp>-<task-id>.md
```

TUI 中的 `/run` 会连续执行任务，直到没有可运行任务。

### Daemon

daemon 持续监听 `.5hagent/task.md`，并将新增或变更的任务作为 Agent process 执行：

```bash
5hagent daemon
5hagent daemon --poll 2s
```

任务结束后会更新源任务状态。daemon 生成的报告带有 task、process 和时间戳，避免覆盖历史结果：

```text
.5hagent/reports/<task-id>.<process-id>.<timestamp>.md
```

## TUI 命令

| 命令 | 作用 |
| --- | --- |
| `/provider [name]` | 选择 provider 或进行认证 |
| `/model <provider/model>` | 查看或切换当前模型 |
| `/session <new\|list\|id>` | 管理会话 |
| `/task <list\|create\|update\|...>` | 管理项目任务 |
| `/run` / `/stop` | 执行或停止文件任务 |
| `/skill <list\|get\|reload>` | 查看或重新加载 Skill |
| `/mcp <list\|add\|remove\|...>` | 管理 MCP 配置 |
| `/compress` | 手动压缩当前上下文 |
| `/detach` | 退出 TUI，保留后台 Agent |

输入 `/` 可以查看命令提示；输入命令前缀后按 Tab 可补全。

## 项目结构

```text
cmd/5hagent/       CLI 入口
internal/
├── runtime/       TUI、headless 和 daemon 的共享装配层
├── agent/         ReAct 循环与工具调度
├── llm/           LLM 客户端
├── tools/         内置工具与注册表
├── task/          文件任务与状态流转
├── context/       上下文和 session
├── skill/         Skill 加载
├── systemd/       Agent process 调度与控制面
└── tui/           Bubble Tea 客户端
doc/               模块文档
```

## 文档

- [文档索引](doc/0README.md)
- [Runtime](doc/runtime/runtime.md)
- [Agent 主循环](doc/core/agent.md)
- [Context 与 Session](doc/core/context.md)
- [Task](doc/core/task.md)
- [Skill](doc/core/skill.md)
- [Tools](doc/integrations/tools.md)
- [TUI](doc/interface/cli.md)
- [LLM 配置](doc/config/llm.md)
