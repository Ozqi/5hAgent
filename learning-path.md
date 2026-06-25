# Learning Path

本仓库保留原始 `master` 历史，同时提供 5 个 `learn/stage-*` 分支作为循序渐进的阅读入口。`develop` 是后续 baseline 规整分支，不属于稳定学习阶段。Stage 6 当前先以设计文档记录，不对应稳定学习分支。

这些分支根据 README 和 TODOLIST 中的开发主线整理，只指向既有里程碑提交，不改写提交历史。

## 使用方式

```bash
git switch learn/stage-1-core-agent
git switch learn/stage-2-tools-task
git diff learn/stage-1-core-agent..learn/stage-2-tools-task
git log --oneline --reverse learn/stage-1-core-agent..learn/stage-2-tools-task
```

如果当前工作区有未提交改动，先提交、stash，或只用 `git diff` 查看，避免切分支时覆盖本地修改。

## 阶段说明

这些入口当前使用分支而不是 Git tag。分支便于后续微调说明文档；如果需要教学 checkpoint 完全不可变，后续可在同一 commit 上补 `learn-stage-*` tag。

## 阶段索引

| 分支 | 关注点 | 建议阅读 |
| --- | --- | --- |
| `learn/stage-1-core-agent` | 最小 ReAct Agent：入口、LLM、主循环 | `cmd/5hagent/main.go`, `internal/agent/`, `internal/llm/` |
| `learn/stage-2-tools-task` | 工具执行扩展：文件工具、并发、流式工具执行、TaskList | `internal/tools/`, `internal/agent/tool_use.go`, `internal/task/` |
| `learn/stage-3-skill-prompt` | Skill、Prompt、文档化：可复用工作流和提示词管理 | `internal/skill/`, `internal/tools/skill_tool.go`, `internal/utils/`, `prompt/`, `doc/` |
| `learn/stage-4-mcp-session-tui` | 外部能力和产品化：MCP、配置、session、TUI、日志 | `internal/mcp/`, `internal/commands/`, `internal/context/`, `internal/cli/`, `internal/logger/` |
| `learn/stage-5-current` | 当前完整实现：安装、MCP 文档、流式稳定性、当前 README | 全仓库 |
| Stage 6 设计 | Agent Systemd：把 Agent 当作进程，由 AI 无关的调度器管理 | `doc/agent-systemd.md` |

## 推荐阅读顺序

1. `stage-1-core-agent`：先看一个最小 Agent 如何跑起来。
2. `stage-2-tools-task`：再看 Agent 如何调用本地工具，并用 TaskList 承载持续任务。
3. `stage-3-skill-prompt`：理解 Prompt 和 Skill 怎么把能力沉淀为可复用上下文。
4. `stage-4-mcp-session-tui`：理解 MCP 外部工具、会话恢复、TUI 和日志如何接入。
5. `stage-5-current`：回到当前实现，看稳定性修复和工程化收尾。
6. `doc/agent-systemd.md`：阅读下一阶段的顶层调度设计；此阶段尚未落成稳定分支。

## 常用对比命令

```bash
git diff learn/stage-1-core-agent..learn/stage-2-tools-task -- internal/agent internal/tools internal/task
git diff learn/stage-2-tools-task..learn/stage-3-skill-prompt -- internal/skill prompt doc
git diff learn/stage-3-skill-prompt..learn/stage-4-mcp-session-tui -- internal/mcp internal/context internal/cli internal/logger
git log --oneline --reverse learn/stage-4-mcp-session-tui..learn/stage-5-current
```
