# Context Spec

## 职责

`internal/context` 管理当前会话的 message context、可选 session store、压缩和上下文元数据。它提供 Agent 读写消息的唯一入口，并把可编辑上下文能力暴露给 `context.context` 工具。

当前 Context 模型记录的是“模型实际可见的消息”。用户可见全量历史、Mask/Projection 和灰色隐藏段仍属于待实现设计，应放在 `.TODO/`，实现前不要写进当前能力描述。

## 覆盖文件

| 文件 | 职责 |
| --- | --- |
| `ctx.go` | `Context`、`Manager`、inspect/pin/edit/audit/compress、tool runtime 注入。 |
| `session.go` | session JSONL 创建、读取、追加、整体替换和会话列表。 |

## 上游和下游

| 方向 | 模块 | 关系 |
| --- | --- | --- |
| 上游 | `internal/agent` | 每轮读写 messages，自动压缩，注入 tool runtime。 |
| 上游 | `internal/tools/context_tool.go` | 调用 inspect/pin/edit/audit/compress。 |
| 上游 | `internal/commands/compress.go` | 手动压缩并输出压缩后消息。 |
| 下游 | `internal/utils` | 读取 `prompt/compress.md`。 |
| 下游 | Eino model | `LMCompress` 调 `Generate` 生成摘要。 |
| 持久化 | `Store` | 可选写入 `~/.walle/sessions/*.jsonl`。 |

## 入口接口

| 接口 | 输入 | 输出 | 行为 |
| --- | --- | --- | --- |
| `NewManager(sessionDir...)` | 可选 session 目录 | `*Manager` | 有目录时启用自动 session 绑定。 |
| `NewMemoryManagerWithStore(sessionDir)` | session 目录 | `*Manager` | 默认 context 走内存，仍允许显式 session 操作。 |
| `CreateContext(sessionID)` | session id 可空 | `*Context` | 创建或恢复 context；autobind 时加载 session 消息。 |
| `GetMessages(ctx)` | Context | 消息副本 | 返回当前消息快照。 |
| `AddMessage(ctx,msg)` | 单条消息 | error | 先写内存，再追加 Store。 |
| `ReplaceMessages(ctx,msgs)` | 完整消息列表 | error | 替换内存，再整体重写 Store。 |
| `SwitchSession(ctx,id)` | 目标 session id | 新 Context | 绑定并读取另一份 session。 |
| `Inspect(ctx)` | Context | `ContextInspect` | 返回索引、角色、长度、预览和 flags。 |
| `PinRange(ctx,range)` | start/end/reason | error | 标记内存 pin，并写 audit。 |
| `EditMessage(ctx,index,content,reason)` | index/content/reason | error | 替换普通 user/assistant 消息并重写 Store。 |
| `Compress(ctx)` | Context | before/after | fallback 压缩，保留 system 和最近历史。 |
| `LMCompress(...)` | LLM、promptDir | before/after | 优先 LLM 摘要，失败时 fallback。 |
| `ManualCompress(...)` | LLM、archiveDir | `CompressResult` | 手动压缩，先归档再替换，失败直接返回。 |

## 数据模型

| 类型 | 关键字段 | 说明 |
| --- | --- | --- |
| `Context` | `messages`、`Session`、`meta` | 一段运行中的消息历史和内存元数据。 |
| `Manager` | `store`、`autobind` | 管理 Context 与 Store 同步。 |
| `ToolRuntime` | `Manager`、`Context` | 通过 Go context 传给 `context.context`。 |
| `ContextMeta` | `Pinned`、`Audit` | 当前进程内的上下文操作元数据。 |
| `ContextRange` | `Start`、`End`、`Reason` | 闭区间消息范围。 |
| `ContextEvent` | `Op`、`Range`、`BeforeCount`、`AfterCount`、`ArchivePath`、`CreatedAt` | pin/edit/compress 审计事实。 |
| `Session` | `ID`、`Title`、`CreatedAt`、`UpdatedAt`、`messages`、`filePath` | 一份 JSONL 会话。 |
| `Store` | `dir`、`cache` | 会话文件目录和进程内缓存。 |

## Session JSONL 格式

每个 session 文件位于 `~/.walle/sessions/<id>.jsonl`。文件是 JSONL 快照：第一行是 session header，后续每行是一条消息。

| 字段 | header | message | 说明 |
| --- | --- | --- | --- |
| `type` | `session` | 空 | 标记 session header。 |
| `id` | 有 | 空 | session id。 |
| `title` | 有 | 空 | 默认 `New Session`，第一条 user 消息会生成短标题。 |
| `created_at` | 有 | 有 | header 或消息时间。 |
| `updated_at` | 有 | 空 | session 更新时间。 |
| `role` | 空 | 有 | `system/user/assistant/tool`。 |
| `content` | 空 | 有 | 消息正文。 |
| `reasoning_content` | 空 | assistant 可有 | thinking/reasoning。 |
| `tool_calls` | 空 | assistant 可有 | assistant 发起的工具调用。 |
| `tool_call_id` | 空 | tool 可有 | 工具结果对应调用 ID。 |
| `tool_name` | 空 | tool 可有 | 工具名。 |
| `extra` | 空 | 可有 | 附加元数据，例如 `created_at`。 |

持久化规则：

- `Append` 会更新内存缓存和更新时间，并调用 `saveToFile` 重写完整文件。
- `ReplaceMessages` 会重写完整文件，适合压缩和编辑后的快照发布。
- `saveToFile` 先写同目录临时文件，再 `os.Rename` 到正式路径。
- 解析旧文件时，未知字段由 JSON 解码忽略，缺失 role 的行跳过。

## 消息生命周期

```text
CreateContext -> ensureConversationSetup -> AddMessage(user)
        -> model call -> AddMessage(assistant/tool)
        -> Compress/ManualCompress -> ReplaceMessages
        -> SwitchSession 可切到另一份 Context
```

稳定约束：

- `GetMessages` 返回切片副本，调用方不能直接改内部切片。
- `AddMessage` 和 `ReplaceMessages` 的 Store I/O 不在 Context 锁内执行。
- Store 写失败时，内存消息已经生效；调用方必须处理返回 error。
- `Message.Extra["created_at"]` 会在落盘前补齐。

## `context.context` 能力

| action | 最小请求 | 行为 | 失败条件 |
| --- | --- | --- | --- |
| `inspect` | `{"action":"inspect"}` | 返回索引、role、字符数、预览、flags。 | 无当前 tool runtime。 |
| `pin` | `{"action":"pin","start":1,"end":3,"reason":"保留需求"}` | 标记闭区间，写 audit。 | 范围非法、缺 reason。 |
| `edit` | `{"action":"edit","index":4,"content":"...","reason":"修正误写"}` | 替换单条 user/assistant 普通消息。 | system/tool/tool-call 消息、pinned 消息、缺 content/reason。 |
| `audit` | `{"action":"audit"}` | 返回最近 20 条审计事件。 | 无当前 tool runtime。 |
| `compress` | `{"action":"compress","mode":"lm"}` | 压缩当前上下文。 | mode 非 `lm/truncate`，或压缩失败。 |

LLM 调用正确率要求：action 枚举少，字段扁平，错误要指出缺哪个字段和允许值。

## Inspect flags

| flag | 含义 |
| --- | --- |
| `protected` | system 消息或最近历史，默认应保留。 |
| `pinned` | 用户或模型显式 pin 的范围。 |
| `compressible` | 当前可被压缩候选命中的普通历史。 |

`inspect` 只返回预览，不返回完整内容，用于降低误把全量上下文二次塞回模型的风险。

## 压缩规则

- `MaxMessages=50`，超过后 `ShouldCompress` 返回 true。
- `KeepRecentMessages=30`，fallback 压缩保留所有 system 和最近 30 条非 system。
- `LMCompress` 读取 `prompt/compress.md`，只把待压缩旧历史发给模型生成摘要。
- LLM 摘要以一条 system message 写回，内容前缀为 `[对话历史摘要]`。
- `LMCompress` 加载 prompt 或模型调用失败时调用 `Compress` fallback。
- `ManualCompress` 会把被压缩消息写入 `archiveDir`，归档成功后再 `ReplaceMessages`。
- `compressWithPrompt` 当前只把 pinned 用于 inspect/edit 保护，未按 pin 调整压缩集合。

## 状态边界

- 当前没有持久化 Projection/Mask 层。
- session JSONL 保存压缩/编辑后的当前快照，不保存完整原始历史版本链。
- pinned range 和 audit event 只在内存中维护，不随 session 恢复。
- Pin 阻止显式 edit；自动压缩仍按当前压缩实现处理。
- Context 不理解任务、report、worklog、daemon process 或 TUI 展示。

## 错误处理

- nil Context 返回 `context is nil`。
- 非法范围返回 `invalid range <start>..<end> for <count> messages`。
- 编辑 system/tool/含 ToolCalls 的 assistant message 会返回明确错误。
- Store 创建、读取、写入和 rename 错误要包装路径语义。
- 手动压缩归档失败时保留原消息，不执行替换。

## 禁止

- 绕过 Manager 直接修改 `Context.messages`。
- 在 Context 层解析 slash command、task、daemon 或工具业务。
- 把 session JSONL 写成多套互不兼容协议。
- 在压缩时丢弃 system prompt。
- 在当前实现完成前把 Mask/Projection 写入对外能力说明。

## 修改检查

- 改消息写入：检查内存、Store cache、JSONL 文件三者是否一致。
- 改 session 格式：用旧 session 文件验证可读性。
- 改压缩：检查 system 消息、最近消息、摘要 message、归档文件、Store 替换。
- 改 pin/edit：检查 pinned 保护、audit 记录、非法 index 错误。
- 改 context tool：检查 `Agent.RunStream` 的 `WithToolRuntime` 注入路径。
- 改 Projection/Mask 设计：先写 `.TODO/`，明确用户可见历史、持久化真源、LM 投影三层关系。
