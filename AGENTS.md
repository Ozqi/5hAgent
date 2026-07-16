# AGENTS.md

本文件是本仓库给 agentic contributor 的工作契约。回答、注释和文档优先使用中文；代码标识符、命令、错误文本和外部 API 名称保持原文。

## 项目定位

`5hAgent` 是一个轻量级 Go + Eino Agent runtime。当前 baseline 不再把 TUI 当成唯一入口，而是用 `internal/runtime` 同时支撑 TUI 和无头任务执行。

```text
cmd/5hagent/main.go
  -> internal/runtime
     -> internal/llm
     -> internal/utils
     -> internal/task
     -> internal/context
     -> internal/agent
        -> internal/agent/tool_use.go
        -> internal/skill
     -> internal/tools
     -> internal/commands
  -> internal/cli
```

交互边界：

- TUI：`5hagent` 初始化 runtime 后启动 Bubble Tea。
- Headless：`5hagent run [--task <id>]` 读取项目目录 `.5hagent/task.md`，执行一个 `in_progress` 或 `pending` 任务，并写 `.5hagent/reports/<task-id>.md`。
- Daemon：`5hagent daemon [--poll <duration>]` 启动最小 Agent Systemd 循环，监听 `.5hagent/task.md` 并把 `pending/in_progress` 任务作为 AgentProcess 执行。
- Session：对话消息默认持久化到 `~/.5hAgent/sessions/*.jsonl`。

## 当前代码事实

- 主入口：`cmd/5hagent/main.go`
- 共享运行时：`internal/runtime/runtime.go`
- ReAct 主循环：`internal/agent/agent.go`
- ToolCall 收集和执行：`internal/agent/tool_use.go`
- Context 和压缩：`internal/context/ctx.go`
- Session 持久化：`internal/context/session.go`
- 任务文件：`internal/task/tasklist.go`
- 工具实现：`internal/tools/*.go`
- 工具注册：`internal/tools/registry.go`
- 工具元数据：`internal/toolmeta/toolmeta.go`
- Slash commands：`internal/commands/*.go`
- Prompt 和配置加载：`internal/utils/utils.go`
- Skill 加载：`internal/skill/skill.go`
- 文档目录：`doc/`

## 文档分工

- `README.md`：面向用户，记录安装、快速开始、运行命令和常用测试入口。
- `doc/0README.md`：文档总览，按模块导航到各子文档。
- `doc/*.md`：模块实现说明，记录架构、关键文件、关键函数和当前边界。
- `开发日志.md`：按时间线记录重要改动、取舍和验证结果。
- `AGENTS.md`：给 agentic contributor 的全局项目契约，记录项目事实、分支状态、模块边界和协作规则；不要重复 README 里的完整命令教程。
- `CLAUDE.md`：Claude Code 入口提示词；需要和本文件的协作节奏保持一致，但不必重复完整项目事实。

如果某个运行或测试命令已经在 README 或模块文档中维护，本文件只保留必要指针，不再复制一份。

## 全局配置事实

- LLM 当前模型只用 `LLM_MODEL=provider/model` 选择；`provider` 对应 `LLM_<PROVIDER>_*` 配置块，`model` 原样发送给上游 API。
- `LLM_<PROVIDER>_FORMAT=claude|openai` 表示接口协议，不是 provider 名；API 地址、密钥、token、stream 都绑定在对应 provider 块。
- 本地 Ollama 作为 provider `ollama` 配置，通过 `LLM_MODEL=ollama/<model>` + `LLM_OLLAMA_FORMAT=openai` + `LLM_OLLAMA_BASE_URL=http://localhost:11434/v1` 接入。
- CLI 可用 `--model provider/model` 临时切换完整模型引用；`--llm-format`、`--llm-model` 只临时覆盖当前 provider 的接口格式或模型名。
- Agent 配置包括 `AGENT_NAME`、`AGENT_MAX_TOTAL_TOKENS`、`AGENT_REPEAT_TOOL_LIMIT`、`AGENT_CONTEXT_AUTO_COMPRESS`。
- Prompts 从 `~/.5hAgent/prompt/*.md` 加载；主 prompt 是 `main.md`，模型专用前缀是 `prefix.<provider>.<model-slug>.md`。
- Skills 启动时从 `~/.5hAgent/skills/*/SKILL.md` 和项目 `.5hagent/skills/*/SKILL.md` 加载；项目同名 skill 覆盖全局 skill，Agent 生命周期内不热加载也不动态启停。
- 项目数据目录是当前工作目录下的 `.5hagent/`；用户级配置目录是 `~/.5hAgent/`。
- 需要跑真实 headless/LLM/toolcall 测试时，统一使用 `/Users/bytedance/Proj/5hWorkSpace` 作为测试 workspace，不要再临时散落到 `/private/tmp`。

当前没有 checked-in `Makefile`、`golangci-lint` 配置、Cursor rules 或 Copilot instruction。不要在文档里虚构不存在的 lint 命令。

## 模块设计

初始化链路：

1. `cmd/5hagent/main.go` 解析 CLI 参数，选择 TUI 或 headless。
2. `internal/runtime.New` 统一初始化配置、logger、任务文件、session、LLM、Agent 和本地工具。
3. `tools.NewRegistry().Init` 注册 base/task/skill/sys 工具和 registry 级工具元数据。
4. `toolRegistry.RegisterContextTool` 注册 `context.context`。
5. Runtime 收集当前 registry 的所有 `schema.ToolInfo`，调用 `WithTools` 生成绑定工具后的 model，再注入 Agent。
6. 启动阶段不启动 MCP stdio server，也不等待 MCP 工具注册；`/mcp` 当前只管理配置。

运行链路：

1. `Agent.RunStream` 确保 system prompt 和 enabled skills 已注入。
2. 将当前 `Context` 通过 `WithToolRuntime` 放入 Go context，供 `context.context` 使用。
3. 添加用户消息，按配置判断是否自动压缩。
4. 调用绑定工具后的 LLM stream。
5. `toolCollector` 合并流式 ToolCall 分片。
6. 单 worker 执行工具，并把 assistant tool call 和 tool result 写回 context。
7. 没有工具调用时写入最终 assistant 消息并返回。

持久化边界：

- `.5hagent/task.md` 是 headless task 的真源。
- `.5hagent/reports/<task-id>.md` 是 headless 执行报告。
- `~/.5hAgent/sessions/*.jsonl` 是 TUI/headless message session 存储。
- `ContextMeta` 中的 pinned range 和 audit event 当前只在内存中维护。

## 工具系统约定

工具注册集中在 `internal/tools/registry.go`：

- `tools.NewRegistry().Init(taskList, skillMgr)` 注册 base/task/skill/sys 工具，并重置当前 registry 的工具列表和元数据。
- `toolRegistry.RegisterContextTool(llm, promptDir)` 注册 `context.context`，必须在 `WithTools` 前调用。
- `toolRegistry.RegisterMCPTools(serverName, client, specs)` 注册 MCP 远端工具。
- 包级工具列表兼容入口已移除；runtime 应持有自己的 `tools.Registry` 实例。
- 当前启动链路不调用 `RegisterMCPTools`，避免 TUI/headless 启动等待外部 MCP 进程；需要恢复 MCP 工具执行时应做 lazy 启动或显式连接。

LLM 可见工具当前包括：

- `base.read_file`
- `base.read_md`
- `base.write_file`
- `base.edit`
- `base.glob`
- `base.grep`
- `base.list_dir`
- `base.exec_shell`
- `task.task`
- `skill.skill`
- `context.context`

`mcp.<server>.<tool>` 当前不是默认启动后的 LLM 可见工具；只有后续实现 lazy 启动或显式连接并注册 MCP tools 后才会出现。

当前执行策略：`RunStream` 中 LLM stream 读取和工具 worker 可以重叠；多个工具调用在单个 worker 内仍是串行执行。不要把当前实现描述成“只读工具并行”。

`context.context` 支持 `inspect/pin/audit/compress`。它依赖 `Agent.RunStream()` 通过 `agentctx.WithToolRuntime(ctx, manager, messageCtx)` 注入当前上下文；脱离当前 Agent 上下文直接调用会失败。

## Agent Systemd 当前边界

- `internal/systemd` 是纯调度核心，只依赖标准库；不要在该包重新引入 `agentctx`、`skill`、`task` 等执行层或业务包。
- `ProcessSpec` 只包含 `SystemPrompt` 和 `ExitCondition`；Project、WorkDir、SessionID、工具白名单等执行期细节仍归 runtime 或工具层处理。
- `AgentSystemd.Run(ctx, runner)` 是唯一调度循环入口；当前只执行硬编码 task supervisor 规则，不再包含 decision 升级点。
- `ProcessRunner` 当前签名是 `RunProcess(ctx, proc)`；runtime 只负责执行 AgentProcess，不再注入 IPC。
- `TaskFileEventSource` 属于 `internal/runtime/event_source_task.go` 适配层；`internal/systemd` 只保留通用 `EventSource` 和 `FileEventSource`。
- `task.created` 使用 `TaskCreatedPayload{process_spec, task_id, task_title}`；dispatch 严格解析并拒绝未知字段。
- 非空事件 ID 会进入 `seen` 去重表，避免重复处理；当前没有 retry/max-retry 调度状态。
- `RunProcess` 结束后投递 `process.exited/process.failed`；report/worklog 路径保存在 `AgentProcess`，不再重复放进事件 payload。
- `task.created` 的 `task_id/task_title` 会进入 `AgentProcess.SourceTask`，不进入 `ProcessSpec`；runtime 用它写 process report、worklog，并在进程结束后把源 task 标记为 `completed/failed`。
- Session 持久化归 `runtime/context` 现有 session manager；`sys.session` 不再作为 LLM 可见工具注册，避免 AgentProcess 额外定义一套落盘语义。
- daemon report 使用 `<task-id>.<process-id>.<timestamp>.md`，不要恢复成只用 `agent-<n>.md` 的覆盖式命名。
- daemon stdout 需要保留 process start/completed/failed、task id 和 report path，方便长期运行时判断状态。
- `dispatch` 异步启动失败但尚未创建进程时，需要补发 `process.failed` 事件；已创建进程后的 runner 错误由 `RunProcess` 自己投递失败事件。
- `cmd/5hagent/main.go` 的 `daemon` 子命令是当前最小运行期调用方；systemd 冒烟测试覆盖 process exited 和异步失败事件。

## 上下文和压缩

- 自动压缩由 `AGENT_CONTEXT_AUTO_COMPRESS` 控制，默认 `true`。
- `LMCompress()` 使用运行时注入的 `prompt/compress.md` 做摘要压缩；失败时 fallback 到 `Compress()`。
- `Compress()` 保留全部 system 消息，再保留最近非 system 消息，避免 fallback 丢失 system prompt / skill 注入。
- 压缩后的消息通过 `Manager.ReplaceMessages()` 和 `Store.ReplaceMessages()` 同步内存 context 与 session JSONL。
- pinned range 和 audit event 当前是内存 metadata，不随 session 恢复。

## 分支逻辑

当前分支用途如下：

| 分支 | 用途 |
| --- | --- |
| `learn/stage-1-core-agent` | Stage 1 学习快照：最小 Go + Eino ReAct Agent。 |
| `learn/stage-2-tools-task` | Stage 2 学习快照：工具调用、任务系统、边输出边执行工具。 |
| `learn/stage-3-skill-prompt` | Stage 3 学习快照：Skill 系统、prompt 管理、Skill 注入机制。 |
| `learn/stage-4-mcp-session-tui` | Stage 4 学习快照：MCP、session 持久化、TUI 和日志体验。 |
| `learn/stage-5-current` | Stage 5 学习快照：当前公开 baseline，对齐 `master` / `origin/master`。 |
| Stage 6 设计 | Agent Systemd 顶层调度设计：启动只传 system prompt / exit condition，context 默认视作进程内存；先记录在 `doc/runtime/agent-systemd.md`，尚未对应稳定学习分支。 |
| `master` | 公开稳定 baseline；当前指向 `learn/stage-5-current`。 |
| `develop` | 当前开发主线；在 `master` 之后继续开发 headless runtime、Ollama baseline、context 工具和文档。 |

这些 `learn/stage-*` 分支是递进快照，不要把它们当作长期功能分支随意改写。日常新改动优先落在 `develop`；需要发布稳定 baseline 时再由维护者决定是否合入 `master` 或新增 stage 快照。

## Git 提交规范

提交信息遵循 Conventional Commits 1.0.0：

```text
<type>[optional scope][!]: <description>

[optional body]

[optional footer(s)]
```

关键规则：

- `feat` 表示新增功能，对应 SemVer minor。
- `fix` 表示修复 bug，对应 SemVer patch。
- 其他类型可按实际意图使用，例如 `docs`、`test`、`refactor`、`chore`、`build`、`ci`。
- scope 可选，放在类型后括号内，例如 `feat(runtime): ...`。
- 破坏性变更必须用 `!` 标记，或在 footer 中写 `BREAKING CHANGE: ...`。
- description 使用祈使、简短描述，不以句号结尾。
- 一次提交只表达一个清晰意图；不要把无关代码、文档和格式化混在一起。

## 工作流要求

- 编辑前先读相关代码和文档。
- 优先做最小正确改动，复用现有函数、类型和包结构。
- 不要随意新增 helper 或抽象；只有明显降低复杂度时再加。
- 不要回滚不属于当前任务的工作树改动。
- 日常小步改动不要求同步更新 `doc/`、`README.md`、`开发日志.md` 或本文件；等一组相关改动告一段落后，再统一补齐文档。
- 改文档时要核对路径、命令、工具名和实际文件是否存在；不要为了小改动主动扩散文档范围。
- 默认不要主动提交。只有用户明确要求，或一组改动已经完成并准备交付时，才整理文档并按提交规范统一 commit。
- 默认不要为了每次改动主动运行测试。只有用户明确要求、改动进入阶段收尾、或风险明显需要验证时再运行测试；如果跳过测试，在最终回复里说明未运行。
- TUI 改动的验收以真实 tmux 窗口为准；不要因为每个小改动都启动/重启 TUI 或跑单测。把一组相关 TUI 改动做完后，再统一用当前 `5hagent debug` 或临时 tmux session 人工验收。

## 设计阶段规则

如果用户明确说还在设计阶段：

- 只写函数签名、类型草图和代码注释。
- 不写具体实现。
- 函数头注释要说明用途、参数、会调用什么、主要步骤。
- 函数名短而清楚，能和现有函数区分。

## 实现阶段规则

如果用户要求实现：

- 按当前包结构落代码。
- 优先复用现有函数。
- 保持文件职责聚焦，避免顺手重构。
- Go 代码必须 `gofmt`。

## Go 风格

- 包名短小写。
- 导出名使用 PascalCase，未导出名使用 camelCase。
- Acronym 风格跟随本文件附近代码，不为了风格大面积改名。
- struct 和 JSON tags 保持显式、稳定。
- 工具输入输出优先使用 typed struct。
- 正常流程返回 error，不 panic。
- 用 `fmt.Errorf("...: %w", err)` 包装错误。
- 工具错误要保留文件路径、参数名或实际收到的值，方便 LLM 自我修正。

## 注释和文档

- 导出类型和函数需要 Go leading comment。
- 保留文件内已有中英文混合风格；不要把注释机械翻译一遍。
- 不写复述代码的噪声注释。
- `doc/` 文档保持架构优先、短而准。
- 模块文档优先放结构图、关键文件、关键函数、当前边界。
- 不写和代码不匹配的历史叙述，除非它解释当前维护方式。

## 验证策略

- 日常小步改动不强制测试，也不强制构建。
- 阶段收尾、准备 commit、发布前或用户明确要求时，再按改动范围选择 `go test`、`go build -o 5hagent cmd/5hagent/main.go` 或真实 workspace smoke test。
- TUI 视觉验收优先使用 tmux 真实画面；单测只作为辅助，不替代人工观察。TUI 小改动不要每次都启动，按一组改动统一验收。
- 只改文档时，默认不跑代码测试；必要时只核对相关路径、命令、文件名和工具名。
- 不要声称支持不存在的工具、skill、prompt 或脚本。

## 规则文件状态

- `AGENTS.md` 是当前仓库主 agent 工作契约。
- `CLAUDE.md` 是 Claude Code 入口提示词；涉及工作节奏、测试、文档和提交策略时，需要和本文件保持一致。
- 当前未发现 `.cursor/rules/`、`.cursorrules`、`.github/copilot-instructions.md`。
- 如果后续新增这些规则文件，需要把新增规则同步折叠回本文件，避免多处规则互相漂移。
