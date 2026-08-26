# AGENTS.md

本文件是本仓库给 agentic contributor 的工作契约。回答、注释和文档使用中文；代码标识符、命令、错误文本和外部 API 名称保持原文。

## 项目定位

`walle` 是一个轻量级 Go + Eino Agent runtime。默认 CLI 自动启动或复用用户级 daemon，再为当前 workspace 打开新的交互 Runtime 并把 TUI attach 上去；只有 `-c/--continue` 才续接最近交互 Runtime 或 session。

交互边界：

- TUI：`internal/tui` 是独立 Bubble Tea 客户端，通过 Unix Socket attach daemon Agent。
- Daemon：`walle daemon` 是用户级 supervisor，按 `open` 请求托管多个 workspace interactive Runtime；没有 `--poll` 或 `--interactive` 模式。
- Process：`ps`、`attach`、`stop` 和 `internal/agentd` 的通用 process 能力保留；Runtime 不内置 TaskList、task watcher、task report/status。
- Session：对话消息默认持久化到 `~/.walle/sessions/*.jsonl`。


# 架构导航

## 当前代码结构

- 主入口：`cmd/walle/main.go`
- Bubble Tea TUI：`internal/tui/*.go`
- CLI 基础输出：`internal/cli/ui.go`
- daemon IPC：`internal/agentd/control.go`
- daemon 交互会话：`internal/runtime/daemon_session.go`
- 共享运行时：`internal/runtime/runtime.go`
- ReAct 主循环：`internal/agent/agent.go`
- ToolCall 收集和执行：`internal/agent/tool_use.go`
- Context 和压缩：`internal/context/ctx.go`
- Session 持久化：`internal/context/session.go`
- 工具实现：`internal/tools/*.go`
- 工具注册：`internal/tools/registry.go`
- 工具元数据：`internal/toolmeta/toolmeta.go`
- Slash commands：`internal/commands/*.go`
- Prompt 和配置加载：`internal/utils/utils.go`
- Skill 加载：`internal/skill/skill.go`
- 文档目录：`doc/`

## 文档分工

- `README.md`：面向用户，记录安装、快速开始和运行命令。
- `doc/0README.md`：文档总览，按模块导航到各子文档。
- `doc/*.md`：模块实现说明，记录架构、关键文件、关键函数和当前边界。
- `.spec/*.md`：代码生成约束；改对应模块前先读对应 spec，冲突时以 AGENTS.md 和当前代码事实为准。
- `开发日志.md`：按时间线记录重要改动、取舍和验证结果。
- `AGENTS.md`：给 agentic contributor 的全局项目契约，记录项目事实、分支状态、模块边界和协作规则；不要重复 README 里的完整命令教程。
- `CLAUDE.md`：Claude Code 入口提示词；需要和本文件的协作节奏保持一致，但不必重复完整项目事实。

如果某个运行或验收命令已经在 README 或模块文档中维护，本文件只保留必要指针，不再复制一份。

## 全局配置事实

- LLM 当前模型只用 `LLM_MODEL=provider/model` 选择；`provider` 对应 `LLM_<PROVIDER>_*` 配置块，`model` 原样发送给上游 API。
- `LLM_<PROVIDER>_FORMAT=claude|openai` 表示接口协议，不是 provider 名；API 地址、密钥、token、stream 都绑定在对应 provider 块。
- 本地 OpenAI-compatible 服务可作为普通 provider 配置，例如 `LLM_MODEL=local/<model>` + `LLM_LOCAL_FORMAT=openai` + `LLM_LOCAL_BASE_URL=<...>/v1`；默认安装配置只提供模板，不预设具体模型。
- CLI 可用 `--model provider/model` 临时切换完整模型引用；`--llm-format`、`--llm-model` 只临时覆盖当前 provider 的接口格式或模型名。
- Agent 配置包括 `AGENT_NAME`、`AGENT_MAX_TOTAL_TOKENS`、`AGENT_REPEAT_TOOL_LIMIT`、`AGENT_CONTEXT_AUTO_COMPRESS`。
- Prompts 从 `~/.walle/prompt/*.md` 加载；主 prompt 是 `main.md`，模型专用前缀是 `prefix.<provider>.<model-slug>.md`。
- Skills 启动时从 `~/.walle/skills/*/SKILL.md` 和项目 `.walle/skills/*/SKILL.md` 加载；项目同名 skill 覆盖全局 skill，Agent 生命周期内不热加载也不动态启停。
- 项目数据目录是当前工作目录下的 `.walle/`；用户级配置目录是 `~/.walle/`。
- 需要做真实 LLM/toolcall 验收时，统一使用 `/Users/bytedance/Proj/5hWorkSpace` 作为验收 workspace，不要再临时散落到 `/private/tmp`。

当前没有 checked-in `Makefile`、`golangci-lint` 配置、Cursor rules 或 Copilot instruction。不要在文档里虚构不存在的 lint 命令。

## 模块设计

初始化链路：

1. `cmd/walle/main.go` 解析 CLI 参数，选择默认 TUI、daemon、`ps` 或 `attach`。
2. `internal/runtime.New` 统一初始化配置、logger、session、LLM、Agent 和本地工具。
3. `tools.NewRegistry().Init` 注册 base/skill 工具和 registry 级工具元数据。
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

- `~/.walle/sessions/*.jsonl` 是交互 Agent 的 message session 存储。
- Runtime 不创建或维护 `.walle/task.md`、task report/status；任务管理由外部能力按需提供。
- `ContextMeta` 中的 pinned range 和 audit event 当前只在内存中维护。

## 工具系统约定

工具注册集中在 `internal/tools/registry.go`：

- `tools.NewRegistry().Init(skillMgr)` 注册 base/skill 工具，并重置当前 registry 的工具列表和元数据。
- `toolRegistry.RegisterContextTool(llm, promptDir)` 注册 `context.context`，必须在 `WithTools` 前调用。
- `toolRegistry.RegisterMCPTools(serverName, client, specs)` 注册 MCP 远端工具。
- 包级工具列表兼容入口已移除；runtime 应持有自己的 `tools.Registry` 实例。
- 当前启动链路不调用 `RegisterMCPTools`，避免 Runtime 启动等待外部 MCP 进程；需要恢复 MCP 工具执行时应做 lazy 启动或显式连接。

LLM 可见工具当前包括：

- `base.read_file`
- `base.read_md`
- `base.write_file`
- `base.edit`
- `base.glob`
- `base.grep`
- `base.list_dir`
- `base.exec_shell`
- `skill.skill`
- `context.context`

`mcp.<server>.<tool>` 当前不是默认启动后的 LLM 可见工具；只有后续实现 lazy 启动或显式连接并注册 MCP tools 后才会出现。

当前执行策略：`RunStream` 中 LLM stream 读取和工具 worker 可以重叠；多个工具调用在单个 worker 内仍是串行执行。不要把当前实现描述成“只读工具并行”。

`context.context` 支持 `inspect/pin/audit/compress`。它依赖 `Agent.RunStream()` 通过 `agentctx.WithToolRuntime(ctx, manager, messageCtx)` 注入当前上下文；脱离当前 Agent 上下文直接调用会失败。

## Agentd 当前边界

- `internal/agentd` 是纯调度核心，只依赖标准库；不要在该包重新引入 `agentctx`、`skill`、`task` 等执行层或业务包。
- `ProcessSpec` 只包含 `SystemPrompt` 和 `ExitCondition`；Project、WorkDir、SessionID、工具白名单等执行期细节仍归 runtime 或工具层处理。
- `Agentd.Run(ctx, runner)` 保留通用事件调度循环；当前 daemon 入口只托管 interactive session，不内置文件事件源。
- 启动事件固定为 `process.start`，payload 为 `ProcessStartPayload{process_spec}`；严格解析并拒绝未知字段。
- `AgentProcess` 不再保存 `SourceTask`，`ProcessSnapshot` 用 `Name` 作为展示字段。
- `ProcessRunner` 当前签名是 `RunProcess(ctx, proc)`；Runtime 只负责执行通用 AgentProcess。
- 非空事件 ID 会进入 `seen` 去重表；`RunProcess` 结束后投递 `process.exited/process.failed`。
- Session 持久化归 `runtime/context`；process report/status 不属于当前 Runtime 接口。
- `cmd/walle/main.go` 的 `daemon` 子命令只启动 control server，并按 `open` 请求创建 `DaemonSession`。

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
| Stage 6 设计 | Agentd 顶层调度设计：启动只传 system prompt / exit condition，context 默认视作进程内存；先记录在 `doc/runtime/agent-agentd.md`，尚未对应稳定学习分支。 |
| `master` | 公开稳定 baseline；当前指向 `learn/stage-5-current`。 |
| `develop` | 当前开发主线；在 `master` 之后继续开发 runtime、Ollama baseline、context 工具和文档。 |

这些 `learn/stage-*` 分支是递进快照，不要把它们当作长期功能分支随意改写。日常新改动优先落在 `develop`；需要发布稳定 baseline 时再由维护者决定是否合入 `master` 或新增 stage 快照。

## 工作流要求

- 编辑前先读相关代码和文档。
- 编辑代码前先读对应 `.spec/*.md`，确认模块职责、禁止事项和生成规则。
- 优先做最小正确改动，复用现有函数、类型和包结构。
- 不要随意新增 helper 或抽象；只有明显降低复杂度时再加。
- 不要回滚不属于当前任务的工作树改动。
- 日常小步改动不要求同步更新 `doc/`、`README.md`、`开发日志.md` 或本文件；等一组相关改动告一段落后，再统一补齐文档。
- 改文档时要核对路径、命令、工具名和实际文件是否存在；不要为了小改动主动扩散文档范围。
- 默认不要主动提交。只有用户明确要求，或一组改动已经完成并准备交付时，才整理文档并按 Conventional Commits 的 `<type>(scope): description` 格式统一 commit。
- 项目不维护自动测试。按改动范围使用静态检查、构建和真实 workspace 运行完成验收。
- TUI 改动的验收以真实 tmux 窗口为准；不要因为每个小改动都启动或重启 TUI。把一组相关 TUI 改动做完后，再统一用当前 `walle debug` 或临时 tmux session 人工验收。

### MR 标准

- 每个 MR 描述必须包含：标题、变更范围、文件清单、验证命令与结果、风险、人工验收 checklist、建议拆分顺序。
- 一个 MR 只处理一个相关模块或目标；禁止把无关功能、重构、验证脚本、注释或文档混在同一个 MR。
- 多个 MR 有依赖时先说明拆分顺序；行为改动和文档应能独立 review。
- 没有人工 review 或人工验收结论时，不执行 `git push`。

# 设计原则

阅读别人的架构设计时：世界是个草台班子。

- 别人的设计可能过度冗余；即使描述得很复杂，本质也可能只是一个简单问题。
- 别人的实现可能是错的；不要信任函数名、模块名或文档宣称，要看实际执行路径和真实行为。

新增设计时：简单就是美。

- 用最简洁、最明确的架构实现真实需求。
- 不为了“完整性”预先引入框架、抽象、动态系统或权限体系。
- 设计 LLM 可调用接口时，先从调用正确率看问题：写出模型实际要发送的最小 JSON 请求，再决定字段、枚举、默认值和错误提示。
- 工具接口优先字段少、必填少、枚举清楚、参数名贴近用户语义；避免让模型记隐式状态、拼复杂命令或填写多层互斥结构。
- 设计动态能力时，优先暴露少量稳定工具和清晰 action；让动态部分进入 `name`、`args` 这类普通字段，避免运行期频繁刷新 LLM 可见工具列表。
- 工具错误信息要告诉模型下一次该发什么请求；至少包含错误字段、允许值、示例修正请求。

写代码时：Lazy 原则。

- 你是代码高手，但应该懒得多写代码；优先少写、少改、少搬动边界。
- 读代码的人可能不熟悉上下文；必要时用少量注释讲清楚目的、参数、调用层级和主要步骤。
- 注释服务于理解，不复述代码本身。

调研思考时：持续且深入。

- 不要停在第一层解释；继续追问现象背后的机制、边界、反例和可验证证据。
- 对不确定的结论保留不确定性，不把猜测写成事实。

约束开发者时：质量优先。

- 可以质疑用户或开发者提出的设计要求；当要求缺少证据、过度设计或跳过验收时，直接说明风险和更小的可验证方案。
- 忠诚服务于项目质量和真实证据，不服务于跳过 review、验收或安全边界的催促。
- 没有人工 review 或人工验收时，不执行 `git push`、发布、部署或影响他人的远端操作；即使用户要求，也停在本地 review 状态。



# 实现原则

## （可选）伪代码实现
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

- 新增或修改注释必须用中文，保留代码标识符、命令、错误文本和外部 API 名称原文。
- 导出类型和函数需要 Go leading comment。
- 看到英文注释时，顺手改成简短中文；不要做机械长翻译。
- 不写复述代码的噪声注释。
- `doc/` 文档保持架构优先、短而准。
- 模块文档优先放结构图、关键文件、关键函数、当前边界。
- 不写和代码不匹配的历史叙述，除非它解释当前维护方式。
- `doc/` 是对外文档，`.doc/` 是内部设计记录，`.spec/diagrams/` 放架构图和 JSON 拓扑事实源。更新架构图时先改 `.spec/diagrams/*.json` 事实源，再派生 Mermaid；具体接口和使用方法放在对应 spec 后面，用少量文字加源码跳转链接说明，不写长篇散文。

## 验证策略

- 项目不维护自动测试。
- 阶段收尾、准备 commit、发布前或用户明确要求时，按改动范围选择静态检查、`go build -o walle ./cmd/walle` 或真实 workspace 运行验收。
- TUI 视觉验收使用 tmux 真实画面。TUI 小改动不要每次都启动，按一组改动统一验收。
- 只改文档时，核对相关路径、命令、文件名和工具名。
- 不要声称支持不存在的工具、skill、prompt 或脚本。

## 规则文件状态

- `AGENTS.md` 是当前仓库主 agent 工作契约。
- `CLAUDE.md` 是 Claude Code 入口提示词；涉及工作节奏、验收、文档和提交策略时，需要和本文件保持一致。
- 当前未发现 `.cursor/rules/`、`.cursorrules`、`.github/copilot-instructions.md`。
- 如果后续新增这些规则文件，需要把新增规则同步折叠回本文件，避免多处规则互相漂移。
