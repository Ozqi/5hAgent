# Context - 上下文管理

## 架构

```mermaid
flowchart TB
    subgraph Context["Context 结构"]
        ctx["Context<br/>messages[]"]
    end

    subgraph Manager["Manager"]
        create["CreateContext()"]
        clone["CloneContext()"]
        add["AddMessage()"]
        get["GetMessages()"]
        clear["Clear()"]
        should["ShouldCompress()"]
        compress["Compress()"]
        lmcompress["LMCompress()"]
    end

    subgraph Compression["压缩"]
        split["splitMessages()"]
        format["formatMessagesForCompression()"]
        archive["writeCompressArchive()"]
    end

    create --> ctx
    clone --> ctx
    add --> ctx
    get --> ctx
    clear --> ctx
    should --> compress
    compress --> split
    lmcompress --> split
    split --> format
    format --> archive
```

## 位置

- `internal/context/ctx.go`

## 核心类型

```go
type Context struct {
    messages []*schema.Message  // 消息列表
}

type Manager struct{}  // 无状态管理器

type CompressResult struct {
    Before      int    // 压缩前消息数
    After       int    // 压缩后消息数
    ArchivePath string // 归档文件路径
}
```

## 常量

| 常量 | 值 | 说明 |
|------|-----|------|
| `MaxMessages` | 50 | 触发压缩的阈值 |
| `KeepRecentMessages` | 30 | 压缩后保留的最近消息数 |

## Manager 接口

| 方法 | 说明 |
|------|------|
| `NewManager()` | 创建无状态 manager |
| `CreateContext()` | 创建空上下文 |
| `CloneContext(parent)` | 克隆父上下文 |
| `GetMessages(ctx)` | 获取所有消息 |
| `AddMessage(ctx, msg)` | 追加消息 |
| `Clear(ctx)` | 清空消息 |
| `ShouldCompress(ctx)` | 检查是否需要压缩 |
| `Compress(ctx)` | 简单截断压缩 |
| `LMCompress(ctx, llm, promptDir)` | LLM 压缩（摘要替换旧消息） |
| `ManualCompress(ctx, llm, promptDir, archiveDir)` | 手动压缩并归档 |

## 压缩流程

### 简单压缩

```go
func (m *Manager) Compress(ctx *Context) (int, int, error) {
    if len(ctx.messages) <= MaxMessages {
        return len(ctx.messages), len(ctx.messages), nil
    }
    keepStart := len(ctx.messages) - KeepRecentMessages
    ctx.messages = ctx.messages[keepStart:]
    return beforeCount, len(ctx.messages), nil
}
```

### LLM 压缩

```go
func (m *Manager) LMCompress(ctx context.Context, ctx *Context, llm model.ToolCallingChatModel, promptDir string) (int, int, error) {
    compressPrompt, _ := utils.Load(promptDir, "compress")
    compressed, _, _, err := m.compressWithPrompt(ctx, ctx, llm, compressPrompt)
    ctx.messages = compressed
    return before, len(ctx.messages), nil
}
```

流程：
1. 分离 system messages 和 history messages
2. 保留最近的 KeepRecentMessages 条历史消息
3. 将早期消息格式化为压缩 prompt
4. 调用 LLM 生成摘要
5. 用摘要消息替换早期消息

### 消息分离

```go
func splitMessages(messages []*schema.Message) (systemMsgs, historyMsgs []*schema.Message) {
    for _, msg := range messages {
        if msg.Role == schema.System {
            systemMsgs = append(systemMsgs, msg)
        } else {
            historyMsgs = append(historyMsgs, msg)
        }
    }
}
```

## 与 Agent 的集成

Agent.RunStream 中的压缩检查（[agent.go:233-239](internal/agent/agent.go)）：

```go
if a.ctxManager.ShouldCompress(messageCtx) {
    before, after, err := a.ctxManager.LMCompress(ctx, messageCtx, a.model, "prompt")
    logger.DebugTag("CTX", "Context compressed: %d -> %d messages", before, after)
}
```

## 消息格式

压缩后的摘要消息格式：

```text
[对话历史摘要]
<LLM 生成的摘要内容>
```

归档文件格式（[ctx.go:220-238](internal/context/ctx.go)）：

```markdown
# Compressed Context Archive

## Summary
<摘要>

## Archived Messages
- role: user
  content: ...
- role: assistant
  content: ...
```

## 当前实现边界

- 不区分 system/user/assistant/tool 消息的重要性
- 不保留工具调用的中间结果
- 不做持久化分层存储
- 简单压缩不做 token 级别的预算控制

## 相关代码

- [ctx.go](../internal/context/ctx.go)
- [agent.go](../internal/agent/agent.go)
- [compress.go](../internal/commands/compress.go)
