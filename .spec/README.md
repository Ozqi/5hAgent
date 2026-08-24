# walle Spec 总览

Spec 是给后续开发者和 Agent 使用的代码描述、架构图与模块契约。它记录职责、入口、状态边界、协议、不变量、禁止事项和验收点，也可以包含待开发设计；待开发内容必须显式标注状态，并以 `.TODO/` 或 `.doc/` 中的设计事实为依据。

本目录追求两件事：

1. 让 Agent 改代码前能快速找到模块边界。
2. 让 review 时能判断改动是否破坏当前架构。

## 写作口径

Spec 长度不设固定上限，按模块代码复杂度和设计边界决定；优先保证描述完整、可执行、可核验。

一份合格 Spec 至少回答：

| 问题 | 要写清的内容 |
| --- | --- |
| 模块负责什么 | 用 2-5 条职责描述，不写愿景口号。 |
| 外部怎么进入 | 列出命令、函数、数据文件或协议帧。 |
| 依赖谁 | 写清调用方向和允许依赖。 |
| 持有什么状态 | 区分内存状态、用户级持久化、项目级持久化。 |
| 稳定约束是什么 | 写不变量、错误处理、并发/顺序保证。 |
| 改动怎么验收 | 给出最小可验证入口，文档改动以路径/事实核对为主。 |

## 拆分原则

- 按架构模块拆分，避免按 Go 包机械拆文件。
- 核心模块写细：Runtime、Agent、Context、Capability、Daemon、Model/Config/Logger。
- 薄模块合并：CLI/TUI 合到 Entry；Skill 合到 Knowledge；Tools/MCP/Commands/ToolEvent 合到 Capability。
- 同一份 Spec 先写稳定协议和边界，再写关键函数；普通 helper 只在影响行为时出现。
- 已实现行为以代码为真源；待开发行为以明确标注的 `.TODO/`、`.doc/` 设计为蓝本。发现冲突时同步修 Spec 或标明待核验。

## 模块索引

| Spec | 覆盖范围 | 核心问题 |
| --- | --- | --- |
| [entry.md](entry.md) | `cmd/walle`、`internal/cli`、`internal/tui` | 用户如何进入系统，TUI/CLI 如何展示和转发。 |
| [runtime.md](runtime.md) | `internal/runtime` | Runtime 如何装配配置、LLM、Agent、工具、状态和 daemon session。 |
| [agent.md](agent.md) | `internal/agent` | ReAct 主循环、ToolCall 收集、工具执行和消息写回。 |
| [context.md](context.md) | `internal/context` | 消息、session JSONL、压缩、上下文元数据。 |
| [capability.md](capability.md) | `internal/tools`、`internal/toolmeta`、`internal/toolevent`、`internal/mcp`、`internal/commands` | LLM 可调用工具、工具事件、MCP 和 slash commands。 |
| [knowledge.md](knowledge.md) | `internal/skill` | Skill 知识加载与注入快照。 |
| [daemon.md](daemon.md) | `internal/systemd`、daemon control 协议 | AgentProcess 调度和 Unix Socket 控制面。 |
| [model-config.md](model-config.md) | `internal/llm`、`internal/codex`、`internal/utils`、`internal/logger` | provider/model 配置、prompt、Codex OAuth、模型 adapter 和进程日志。 |

## 架构图

架构图和拓扑事实源集中放在 `diagrams/`：

| 文件 | 用途 |
| --- | --- |
| `diagrams/walle-architecture-topology.json` | walle 架构拓扑事实源，供 Mermaid 分模块作图。 |
| `diagrams/walle-overall-runtime.mmd` | walle 总览图 Mermaid。 |
| `diagrams/walle-daemon-control.mmd` | daemon 控制面图。 |
| `diagrams/walle-interactive-runtime.mmd` | 默认 TUI、daemon、Runtime 交互时序图。 |
| `diagrams/walle-agent-turn.mmd` | 单轮 ReAct 时序图。 |
| `diagrams/walle-context-projection.mmd` | 上下文投影图。 |
| `diagrams/walle-capabilities.mmd` | 能力挂载图。 |

## 全局边界

- 默认交互入口是可分离 TUI：CLI 拉起或复用固定的 `daemon`，再通过 Unix Socket attach。
- `Runtime` 是装配层；具体工具行为在 Capability，调度在 Daemon，渲染在 Entry/UI。
- Runtime 不内置 TaskList、`walle run`、task watcher 或任务 report/status；任务管理由 Skill、MCP 或外置动态工具提供。
- Session 默认写 `~/.walle/sessions/*.jsonl`。
- 默认启动只注册本地工具和 `context.context`，MCP 需要显式连接后才注册远端工具。
- 模型选择统一用 `LLM_MODEL=provider/model` 或 CLI `--model provider/model`。
- 注释和文档使用中文；代码标识符、命令、错误文本和外部 API 名称保持原文。

## Spec 和其它文档的分工

| 文档 | 用途 |
| --- | --- |
| `README.md` | 用户安装、启动、常用命令。 |
| `doc/` | 对外架构说明和模块导读。 |
| `.doc/` | 内部调研、决策记录、推演记录。 |
| `.TODO/` | 待实现设计和计划。 |
| `.spec/` | 代码描述、架构图、拓扑事实源、待开发设计和模块契约；待开发内容显式标注状态。 |
| `AGENTS.md` / `CLAUDE.md` | Agent 协作规则、项目事实、提交流程。 |

## 依赖方向

```text
Entry/UI -> Runtime -> Agent -> Context
                  |       |-> Capability -> Knowledge
                  |       |-> Model/Config
                  |-> Daemon adapter -> Systemd core
```

允许的方向：

- `cmd/`、`internal/tui` 可以调用 Runtime、Daemon client、Commands。
- Runtime 可以组合 Agent、Context、Capability、Knowledge、Model/Config 和 Daemon adapter。
- Agent 可以调用 Context、Skill manager、Tool instances、ToolEvent、Logger。
- Capability 可以调用 Skill/MCP/Context 的窄接口。
- `internal/systemd` 只依赖标准库和本包类型。
- `internal/logger` 是低层横切能力，禁止反向依赖业务包。

## 改动前检查

1. 先读目标模块 Spec。
2. 再读对应 `doc/` 模块文档。
3. 最后读代码入口和调用方。
4. 如果新需求属于未来能力，先在 `.TODO/` 或 `.doc/` 固化设计，再写入 `.spec/` 并标注 `planned`。
5. 如果只改一个薄模块，优先更新所属大模块 Spec，避免新增零散 Spec 文件。

## 验收策略

- 项目不维护自动测试。
- 文档-only 改动：核对路径、命令、模块名存在。
- Go 代码改动：按模块选择静态检查、`go build -o walle ./cmd/walle` 或真实运行验收。
- TUI 改动：用真实 tmux 窗口或 attach 流程观察。
- daemon/control 改动：检查 socket 生命周期、attach 握手、断线语义和进程状态。
- 模型/provider 改动：优先使用 fake server 或短超时请求验证错误路径，避免真实额度消耗。
- 人工 review 前不执行 `git push`、发布、部署或影响他人的远端操作。

## 覆盖审计清单

当前 Go 包应被上述 8 份 Spec 覆盖：

- `cmd/walle`
- `internal/agent`
- `internal/cli`
- `internal/codex`
- `internal/commands`
- `internal/context`
- `internal/llm`
- `internal/logger`
- `internal/mcp`
- `internal/runtime`
- `internal/skill`
- `internal/systemd`
- `internal/toolevent`
- `internal/toolmeta`
- `internal/tools`
- `internal/tui`
- `internal/utils`
