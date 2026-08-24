# Agent Spec

## 职责

`internal/agent` 是 ReAct 执行核心。它负责把当前消息上下文送入模型，收集模型输出中的 ToolCall，执行工具，把 assistant/tool result 写回 context，并把 token、thinking、工具事件向上层输出。

Agent 的职责保持收敛：

- 管理一轮或多轮 ReAct loop 的顺序。
- 确保 system prompt 和已加载 skills 注入到空 context。
- 统一处理流式和非流式模型输出。
- 执行已注册工具并生成模型可继续理解的 tool result。
- 统计 token 和当前 turn，供 TUI、daemon、日志展示。

## 覆盖文件

| 文件 | 职责 |
| --- | --- |
| `agent.go` | `Agent`、配置、状态、主循环、skill 注入、token 预算、模型/工具替换入口。 |
| `tool_use.go` | ToolCall 分片合并、工具执行、tool result 写回、工具错误提示。 |
| `callbacks.go` | LLM/tool 调试回调、token usage 统计。 |
| `debug_request.go` | `--debug` 下记录发给 LLM 的逻辑请求，并对敏感字段做掩码。 |

## 上游和下游

| 方向 | 模块 | 关系 |
| --- | --- | --- |
| 上游 | `internal/runtime` | 创建 Agent，注入模型、工具、Context Manager、tool event sink。 |
| 上游 | `internal/tui` / daemon session | 调用 `RunStream`，消费 token 和 thinking 回调。 |
| 下游 | `internal/context` | 所有消息读写通过 `Manager`。 |
| 下游 | `internal/tools` | 只通过 `tool.BaseTool` / `tool.InvokableTool` 调用。 |
| 下游 | `internal/skill` | 读取启动时 skill snapshot。 |
| 下游 | `internal/toolevent` / `internal/logger` | 输出工具事件和 debug 日志。 |

## 入口接口

| 接口 | 输入 | 输出 | 行为 |
| --- | --- | --- | --- |
| `NewAgent(model, tools, config)` | Eino model、工具列表、`Config` | `*Agent` | 补默认配置、加载 skill、建立 tool map、初始化 token budget。 |
| `RunStream(ctx, messageCtx, input, onToken, onReasoning...)` | 用户输入和回调 | 最终 assistant 文本 | 调用 `RunStreamWithOptions`，使用当前模型默认 options。 |
| `RunStreamWithOptions(..., opts, ...)` | 临时模型 options | 最终 assistant 文本 | 执行完整 ReAct loop，opts 只影响本轮。 |
| `SetModel` / `SetTools` | 新模型或工具列表 | 无 | Runtime 切换模型或重绑工具后更新 Agent。 |
| `SetCtxManager` | Context Manager | 无 | Runtime 注入持久化或内存 context 管理器。 |
| `SetToolEventSink` | 事件回调 | 旧回调 | 让 TUI、daemon、worklog 接管工具事件。 |
| `SetDebugModel` | `provider/model`、secret | 无 | debug request 记录时使用模型名和敏感值掩码。 |
| `CurrentTurn` | 无 | 最近一次 ReAct 轮次 | 供 UI 和 daemon snapshot 展示。 |

## 配置字段

| 字段 | 语义 | 默认或约束 |
| --- | --- | --- |
| `Name` | Agent 名称 | 空时调用方按场景补。 |
| `MaxTotalTokens` | 会话累计 token 上限 | 0 时补 `1000000`。 |
| `RepeatToolLimit` | 相同工具名和参数的重复上限 | 0 时补 `5`。 |
| `Debug` | 调试日志开关 | 影响 callbacks 和 LLM request dump。 |
| `ContextAutoCompress` | 自动上下文压缩 | Runtime 从配置读取。 |
| `DisableStream` | 使用 `Generate` 路径 | 兼容流式不稳定 provider。 |
| `SystemPrompt` | 系统提示词 | `ensureConversationSetup` 注入。 |
| `PromptDir` | prompt 目录 | 供 context 压缩读取 `compress.md`。 |
| `ProjectDataDir` | 项目 `.walle` | skill 加载和项目数据定位。 |

## 主流程

1. `RunStreamWithOptions` 读取 reasoning callback。
2. `ensureConversationSetup` 在空 context 写入 system prompt 和已加载 skills。
3. `agentctx.WithToolRuntime` 把当前 `Manager` 和 `Context` 注入 Go context，供 `context.context` 使用。
4. 写入本轮 user message。
5. 若 `ContextAutoCompress=true` 且 `ShouldCompress` 命中，调用 `LMCompress`。
6. 初始化 `toolRepeatGuard`。
7. 按 ReAct turn 循环读取当前消息快照。
8. 调用模型：流式路径走 `model.Stream`，兼容路径走 `model.Generate`。
9. 收集 assistant 文本、thinking、响应元数据和 ToolCall。
10. 若有 ToolCall，先写 assistant tool-call message，再写每个 tool result message，然后进入下一 turn。
11. 若没有 ToolCall 且有正文，写最终 assistant message 并返回正文。
12. 若没有 ToolCall 且没有正文，最多追加两次 user reminder 继续推进，第三次返回错误。

## 流式 ToolCall 规则

- `toolCollector` 按 ToolCall `Index` 合并分片；没有 `Index` 时归到 0。
- ToolCall ID 为空且工具名存在时生成 `call_local_<index>`。
- 只有工具名非空、参数是合法 JSON 的调用会派发。
- 流式阶段空参数不立即派发；EOF 后空参数归一化为 `{}`。
- 当前返回顺序来自 Go map 遍历后形成的派发顺序；写回时按 `queuedCalls` 下标还原该顺序。
- 不完整 ToolCall 会被过滤，避免写入模型无法匹配的 tool result。

## 工具执行策略

当前实现的执行边界：

- 模型 stream 读取和工具 worker 可以重叠。
- 一个 `Agent` 内只有一个工具 worker。
- 多个工具调用在该 worker 内按队列顺序执行。
- `exeToolCall` 的 `concurrent` 参数当前只影响事件展示，主路径传 `false`。
- 工具执行错误会转成 tool message 交给模型继续自我修正。
- `addToolResult` 返回 error 仅表示写 context 失败。

ToolCall 并发属于独立实现任务。落代码前需要明确：结果顺序、取消传播、副作用工具隔离、重复调用保护、事件展示和验收证据。

## 非流式路径规则

- `DisableStream=true` 时调用 `model.Generate`。
- 生成路径也要处理 ToolCalls、ReasoningContent、ResponseMeta、空响应重试。
- 有 ToolCall 时按模型返回顺序逐个执行并写回。
- 修改 ReAct 语义时必须同步流式和非流式路径。

## 消息写回规则

| 消息 | 写入时机 | 内容 |
| --- | --- | --- |
| system | 空 context 初始化时 | base prompt 和 skill 注入内容。 |
| user | 每次 RunStream 开始 | 用户本轮输入。 |
| assistant with tool calls | 模型要求工具时 | content、reasoning、tool_calls、response meta、extra。 |
| tool | 每个工具执行后 | 工具结果文本，或格式化错误文本。 |
| assistant final | 模型给最终正文时 | content、reasoning、response meta、extra。 |

写入必须走 `ctxManager.AddMessage`。批量替换只允许 context manager 执行。

## Skill 注入规则

- Agent 创建时加载全局和项目 skill snapshot。
- `ensureConversationSetup` 只在空 context 注入 system prompt 与 skill 内容。
- `/skill reload` 更新 manager 后，后续新 context 可看到新 snapshot。
- 已写入当前 context 的 skill system message 保持原样。

## 错误处理

- 模型 stream/generate 错误包装为 `LLM stream failed` 或 `LLM generate failed`。
- 流式读取 90 秒没有新 chunk 时取消 reader 和工具 context，返回 idle timeout。
- 重复工具调用超过限制时写入 tool error message。
- 工具缺失时返回 `tool not found: <name>`，并由 tool error 格式保留原始参数。
- 工具错误提示应包含下一次可修正请求的方向，例如合法 action、绝对路径或 shell command。
- debug request 必须对 API key、token 等敏感值做掩码。

## 状态边界

- `Agent.currentTurn` 只反映最近一次 ReAct loop 轮次。
- Agent 不直接读写 session 文件；消息持久化走 `internal/context.Manager`。
- Agent 不实现任务管理，也不直接写 process report/worklog。
- Agent 不持有 provider 配置真源；模型切换由 Runtime 完成。

## 不变量

- 同一 `Agent` 实例按设计串行执行一轮用户输入。
- 工具索引来自 `SetTools` 注入的工具列表；工具名匹配使用模型可见完整名。
- system prompt 和 skill 注入只发生在空 context。
- 有工具调用时必须继续 ReAct loop，让模型看到工具结果后再输出最终回答。
- token budget 只统计模型回调中的 usage，不自行估算全文 token。

## 禁止

- 在 Agent 中实现 daemon、TUI、任务管理、process report/worklog、provider 登录或配置读取。
- 绕过 Context Manager 直接修改消息切片。
- 在工具错误时丢掉工具名、参数或原始错误。
- 无工具调用后无限继续 ReAct 循环。
- 在未定义顺序和副作用边界前直接把工具 worker 扩成无界 goroutine。

## 修改检查

- 改 ToolCall 收集：检查分片合并、空参数、ID 生成、不完整调用过滤。
- 改工具执行：检查执行顺序、重复调用保护、取消传播、tool result 对应的 `tool_call_id`。
- 改消息写回：检查 session JSONL、Codex adapter 的历史重放、Eino 消息格式。
- 改流式模型路径：同步非流式 `Generate` 路径。
- 改 skill 注入：检查空 context、新 session、已有 session、`/skill reload`。
- 改 debug：确认 secret 掩码，避免把 API key 写进日志或报告。
