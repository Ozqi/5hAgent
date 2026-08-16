# Context

## 职责

`internal/context` 管理模型消息、session JSONL、上下文压缩和运行时元数据。它不理解业务任务，只保存消息和少量审计信息。

## 关键文件

| 文件 | 作用 |
| --- | --- |
| [ctx.go](../../internal/context/ctx.go) | `Context`、`Manager`、压缩、inspect/pin/audit |
| [session.go](../../internal/context/session.go) | `~/.5hAgent/sessions/*.jsonl` 读写 |
| [context_tool.go](../../internal/tools/context_tool.go) | LLM 可调用的 `context.context` |

## 消息生命周期

```text
CreateContext(sessionID)
  -> Store.GetOrCreate
  -> Store.LoadMessages
AddMessage
  -> append memory
  -> Store.Append(session, msg)
ReplaceMessages
  -> replace memory
  -> Store.ReplaceMessages
```

## Session JSONL

每个文件以 session header 开头，后续一行一条消息。

```json
{"type":"session","id":"...","title":"...","created_at":"...","updated_at":"..."}
{"role":"user","content":"...","created_at":"...","extra":{"created_at":"..."}}
```

当前消息字段：`role/content/reasoning_content/tool_calls/tool_call_id/tool_name/created_at/extra`。
旧 session 没有 `created_at` 仍可读取。

## 压缩

| 函数 | 行为 |
| --- | --- |
| `ShouldCompress` | 按消息数量判断是否触发自动压缩。 |
| `LMCompress` | 使用 `prompt/compress.md` 调模型总结；失败 fallback 到 `Compress`。 |
| `Compress` | 简单截断，保留 system 和最近消息。 |
| `ManualCompress` | `/compress` 入口，可写 `compact/messages/*.md`。 |

## context.context

| action | 作用 |
| --- | --- |
| `inspect` | 返回消息索引、角色、预览、保护标记。 |
| `pin` | 标记消息范围，避免被压缩/编辑。 |
| `audit` | 返回 context 操作事件。 |
| `compress` | 触发截断或 LLM 压缩。 |

## 边界

- pinned range 和 audit 只在内存中维护，不随 session 恢复。
- session 是用户会话恢复；daemon/process report 不是 session。
- `WithToolRuntime` 在当前 Agent 运行期间注入消息上下文，供 `context.context` 使用。
