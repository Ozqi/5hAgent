# 上下文掩盖与 LLM 投影

## 一句话

Session 保存完整上下文；Context 是发给 LLM 的可控投影。Agent 只操作 Context 状态，Session 原始历史由系统维护。

## 第一版对齐结论

- 只做整条 message 粒度，先不做 paragraph、byte/rune range 或 TUI 选区。
- `pin` 表示该 message 一直进入 LLM 投影，并阻止后续 `mask`。
- `mask` 表示该 message 保留在 Session 和 TUI 中，下次请求 LLM 时跳过。
- `pin` 和 `mask` 互斥；同一条 message 同时设置时直接返回错误。
- 状态放在 message `Extra["projection"]`，随 session JSONL 一起落盘。
- 工具接口先复用 `context.context`，通过 action 暴露 `pin/unpin/mask/unmask/inspect/audit`。
- `Agent.RunStreamWithOptions` 发起模型请求前调用投影函数，过滤 masked message。
- 第一版不改 TUI 灰色显示、不拆多个 `context.*` 工具、不做非破坏压缩重构。

## 当前代码事实

- `Agent.RunStreamWithOptions` 每轮调用 `Manager.GetMessages(messageCtx)`，拿到的消息会直接传给 `a.model.Stream/Generate`。
- `context.context` 已有 `edit` action，入口在 `internal/tools/context_tool.go`。
- `edit` 调用 `Manager.EditMessage`，会改内存里的 `ctx.messages[index].Content`。
- 如果当前 Context 绑定了 Session，`EditMessage` 会调用 `Store.ReplaceMessages` 重写整个 session JSONL。
- `edit` 的审计事件只在 `ContextMeta.Audit` 内存里，session 恢复后不保留。
- `pinned` 和 `audit` 当前也只在内存中维护，不随 session 恢复。
- 自动压缩和手动压缩当前都会通过 `ReplaceMessages` 改写当前上下文；这和“持久化上下文是 LLM 输入的超集”目标存在张力，需要后续收敛。

## 直接编辑会发生什么

如果模型或用户通过 `context.context` 直接编辑一条消息：

1. 下一轮发给 LLM 的内容会变成编辑后的内容。
2. TUI 和 session 恢复看到的内容也会变成编辑后的内容。
3. 原文不会保留在 session JSONL 中。
4. 只剩内存审计知道发生过 `edit`，进程退出后审计消失。

结论：`edit` 是破坏性历史改写，适合修正错误消息；不适合作为“让 LLM 忽略一段内容”的默认能力。

## 目标状态

- 用户可见历史保持完整，可灰色显示被 LLM 忽略的内容。
- session 持久化保存完整消息和 Context 状态元数据。
- LLM 请求前统一调用投影函数，只发送允许进入模型上下文的 Session 子集。
- 掩盖内容可恢复；取消掩盖后，原文仍可再次进入 LLM 输入。
- 压缩、摘要、debug request 都必须走同一层投影，避免绕过 mask。

## 概念

| 名称 | 含义 |
| --- | --- |
| Session | 用户可见、session 持久化的完整消息历史。 |
| Context / LM Projection | 从 Session 派生出的模型输入消息列表。 |
| Mask | 一条消息仍保存在 Session，但不进入 LM Projection。 |
| Pin | 强保护内容，防止压缩、编辑和掩盖。 |
| Edit | 显式改写历史内容，保留为高风险能力。 |

## mask 语义

第一版只支持整条 message：

| 范围 | 说明 |
| --- | --- |
| message | 整条普通 user/assistant 消息对 LLM 隐藏。 |

后续确实需要精确选择时，再增加 paragraph、byte/rune range 或 TUI 选区。

暂缓的 paragraph 语义可以用空行切分：

```text
段落 A

段落 B

段落 C
```

## 发送给 LLM 的表现

被 mask 的整条 message 不进入 LLM 请求。Session 和 TUI 仍保留原文。

第一版跳过占位消息，保持实现直接：`GetLMMessages` 返回过滤后的消息副本。后续如果真实模型对 role 顺序敏感，再补同 role 占位。

## 数据存储

优先把 mask 元数据放到消息 `Extra`，因为当前 session JSONL 已经持久化 `extra` 字段。

建议结构：

```json
{
  "extra": {
    "projection": {
      "pin": true,
      "mask": false,
      "reason": "用户要求这条一直保留",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  }
}
```

被 mask 的消息：

```json
{
  "extra": {
    "projection": {
      "pin": false,
      "mask": true,
      "reason": "用户要求这条不要再发给 LLM",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  }
}
```

这个方案不需要新增 session sidecar 文件。后续如果 ContextMeta 需要统一持久化，再迁移到 session header 或独立 metadata 文件。

## context.context 工具调整

第一版继续复用当前 `context.context` 工具，靠 `action` 区分操作：

| action | 参数 | 作用 |
| --- | --- | --- |
| `inspect` | 可空 | 查看完整历史索引、当前 `pin/mask` 状态和可操作范围。 |
| `pin` | `index`, `reason` | 标记单条 message 一直进入 Context 投影。 |
| `unpin` | `index` | 取消单条 message 的 pin。 |
| `mask` | `index`, `reason` | 标记单条 message 退出 Context 投影。 |
| `unmask` | `index` | 取消单条 message 的 mask。 |
| `audit` | 可空 | 返回 `pin/unpin/mask/unmask/edit/compress` 等 Context 状态操作。 |

`edit` 保留为修正错误历史的显式改写能力；隐藏、忽略、减少注意力的需求使用 `mask`。

## Manager 接口草图

```go
// MaskMessage 标记单条消息不进入 LLM 投影。
func (m *Manager) MaskMessage(ctx *Context, index int, reason string) error

// UnmaskMessage 取消单条消息的 mask 标记。
func (m *Manager) UnmaskMessage(ctx *Context, index int) error

// GetLMMessages 返回发给模型的投影消息。
func (m *Manager) GetLMMessages(ctx *Context) ([]*schema.Message, error)
```

`Agent.RunStreamWithOptions` 后续改为：

```text
messages := ctxManager.GetLMMessages(messageCtx)
```

TUI 渲染继续使用完整上下文视图。

## 约束

- system 消息第一版不可 mask。
- pinned 消息不可 mask。
- 含 ToolCall 的 assistant 消息第一版不可整条 mask。
- tool result 第一版不可整条 mask；后续如果工具输出过大，优先做摘要投影。
- mask 元数据必须持久化。
- mask/unmask 必须写 audit。
- 自动压缩和手动压缩不能把被 mask 的原文发送给压缩模型。

## TUI 体验

- 后续可把被 mask 的消息用灰色 / dim 样式显示。
- 后续可在行首显示轻量标记，例如：`◌ ignored by LM` 或 `◌ 不发给 LLM`。
- 后续可增加鼠标或键盘选区触发“mask / unmask”。
- 第一版先不做 TUI 特殊渲染和复杂选区 UI；`inspect` 能看到状态即可。

可接受的第一版命令：

```text
context.context {"action":"mask","index":12,"reason":"用户要求这条不要再发给模型"}
context.context {"action":"unmask","index":12}
```

## 压缩边界

当前 `Compress/LMCompress/ManualCompress` 会替换消息列表。新目标下，压缩应该逐步改成 projection 层能力：

- Session 保留原始消息。
- Context / LM Projection 可以包含摘要、占位和最近消息。
- 压缩摘要生成时只读取允许进入 LLM 的内容。
- 归档可以保留，但不能作为 Session 的唯一原文来源。

第一阶段先保证 mask 不被主 LLM 和压缩 LLM 绕过；压缩完全非破坏化可以单独拆 TODO。

## 开发计划

1. 增加 projection 元数据读写，优先挂在 `schema.Message.Extra["projection"]`。
2. 实现 `MaskMessage/UnmaskMessage`，复用并调整 `PinRange` 或补 `PinMessage/UnpinMessage`。
3. 实现 `GetLMMessages`，先按整条 message 过滤 `mask=true`，并返回消息副本。
4. `context.context` 增加 `mask/unmask/unpin`，并更新 `inspect` flags。
4. `Agent.RunStreamWithOptions` 改用 `GetLMMessages`。
5. 压缩路径改用同一投影，避免发送 masked 原文。
6. 增加最小验证：mask 后 session 保留原文，debug request / fake model 输入看不到原文。
7. 后续再做 paragraph mask、TUI 灰色显示和非破坏压缩。

## 验收标准

- 用户消息被 mask 后，session JSONL 仍保存原文。
- 下一轮 LLM 输入不包含被 mask 的原文。
- unmask 后，下一轮 LLM 输入重新包含原文。
- pinned 内容无法 mask，并返回清楚错误。
- `edit` 仍可显式改写普通消息，但工具描述引导“隐藏内容用 mask”。

## Prompt Cache 约束

上下文投影会影响 prompt cache 命中率。主流 provider 的缓存基本依赖“从请求开头开始的稳定前缀”：

- OpenAI：自动复用 exact prompt prefix；动态内容应放在尾部，稳定内容保持顺序不变。
- Anthropic / Bedrock Claude：tools -> system -> messages 的前缀顺序很关键；cache breakpoint 应放在稳定内容末尾。
- 通用规律：tools、system prompt、项目规则、稳定 skill 描述放前面；当前时间、git 状态、最新工具结果、mask 变化放后面。

对 walle 的设计约束：

1. `LM Projection` 必须分层：`stable prefix` + `dynamic tail`。
2. mask/unmask 只改投影元数据；默认不要重写早期消息文本。
3. 动态状态不要插入 system prompt；放进本轮 user message 或 tail block。
4. 压缩不要频繁微调前缀；达到阈值后批量 fork/替换尾部。
5. tools/schema 顺序必须稳定，动态工具注册会天然降低缓存命中率，因此注册后才重绑工具，平时不刷新。
6. debug request 需要显示 cache 相关分层：stable prefix token、dynamic tail token、provider 返回的 cached tokens（如果有）。

第一版实现目标调整：

- 先实现“投影层不改写 Session 原始历史”。
- 再保证“投影输出顺序稳定”：固定 system/tools/project context，动态内容追加到尾部。
- cache breakpoint / prompt_cache_key 作为 provider adapter 能力，等模型接口收敛时再接。
