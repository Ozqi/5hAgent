# Context - 上下文管理

## 位置
`internal/context/ctx.go` (~110行)

## 结构

```go
type Context struct {
    messages []*schema.Message
}

type Manager struct {
    // 无状态，纯函数式管理
}
```

## 常量配置

```go
const (
    MaxMessages        = 50  // 触发压缩的阈值
    KeepRecentMessages = 30  // 压缩后保留的消息数
)
```

**代码链接**: [ctx.go:8-11](../internal/context/ctx.go#L8-L11)

## 核心函数

### NewManager() *Manager
创建上下文管理器（无状态）

### CreateContext() (*Context, error)
创建新的空上下文

### CloneContext(parent *Context) (*Context, error)
克隆上下文（用于 sub-agent）
- 深拷贝消息列表
- 隔离父子上下文

### GetMessages(ctx *Context) ([]*schema.Message, error)
获取上下文中的所有消息

### AddMessage(ctx *Context, msg *schema.Message) error
添加消息到上下文
- 追加到 messages 列表末尾

### Clear(ctx *Context) error
清空上下文
- 重置 messages 为空列表

### ShouldCompress(ctx *Context) bool ⭐ Phase 2 新增
检查是否需要压缩
- 返回 `len(ctx.messages) > MaxMessages`

**代码链接**: [ctx.go:111-113](../internal/context/ctx.go#L111-L113)

### Compress(ctx *Context) (int, int, error) ⭐ Phase 2 新增
压缩上下文，保留最近的消息
- **输入**：Context 实例
- **输出**：压缩前消息数、压缩后消息数、错误
- **逻辑**：
  ```go
  if len(messages) <= MaxMessages {
      return beforeCount, beforeCount, nil
  }
  keepStart := len(messages) - KeepRecentMessages
  ctx.messages = ctx.messages[keepStart:]
  ```

**代码链接**: [ctx.go:91-109](../internal/context/ctx.go#L91-L109)

## 使用场景

### 1. Agent 初始化
```go
ctxManager := agentctx.NewManager()
messageCtx, _ := ctxManager.CreateContext()
```

### 2. 添加消息
```go
// 系统消息
systemMsg := &schema.Message{
    Role:    schema.System,
    Content: "You are a helpful assistant.",
}
ctxManager.AddMessage(messageCtx, systemMsg)

// 用户消息
userMsg := &schema.Message{
    Role:    schema.User,
    Content: "Hello",
}
ctxManager.AddMessage(messageCtx, userMsg)

// 助手消息（带工具调用）
assistantMsg := &schema.Message{
    Role:      schema.Assistant,
    Content:   "",
    ToolCalls: []schema.ToolCall{...},
}
ctxManager.AddMessage(messageCtx, assistantMsg)

// 工具结果消息
toolMsg := schema.ToolMessage("result", "tool_call_id")
ctxManager.AddMessage(messageCtx, toolMsg)
```

### 3. 自动压缩（集成在 Agent.RunStream）
```go
// agent.go:236-243
if a.ctxManager.ShouldCompress(messageCtx) {
    before, after, err := a.ctxManager.Compress(messageCtx)
    if err != nil {
        return "", fmt.Errorf("failed to compress context: %w", err)
    }
    logger.InfoTag("CTX", "Context compressed: %d -> %d messages", before, after)
    fmt.Printf("\n%s\n", logger.Yellow(fmt.Sprintf("[上下文压缩: %d -> %d 条消息]", before, after)))
}
```

## 压缩策略

### 触发条件
- 消息数 > 50 条

### 压缩方式
- **简单截断**：保留最近 30 条消息
- **优点**：实现简单，性能高
- **缺点**：丢失早期上下文

### 未来优化方向
1. **智能压缩**：
   - 保留系统消息
   - 保留关键工具调用结果
   - 压缩中间对话

2. **LLM 总结**：
   - 使用 LLM 总结早期对话
   - 将总结作为新的系统消息
   - 参考 Claude Code 的 compact.ts

3. **分层存储**：
   - 热数据：最近 30 条（内存）
   - 温数据：31-100 条（总结）
   - 冷数据：100+ 条（持久化）

## 消息类型

### schema.Message 结构
```go
type Message struct {
    Role      string      // system, user, assistant, tool
    Content   string      // 消息内容
    ToolCalls []ToolCall  // 工具调用（assistant 消息）
}
```

### 消息流转示例
```
1. System:    "You are a helpful assistant."
2. User:      "读取 task.md"
3. Assistant: "" + ToolCalls[{name: "read_file", args: "..."}]
4. Tool:      "{\"content\": \"...\", \"total_lines\": 58}"
5. Assistant: "文件内容如下：..."
6. User:      "总结一下"
7. Assistant: "总结：..."
```

## 代码位置

- [ctx.go:1-23](../internal/context/ctx.go#L1-L23) - 结构定义和常量
- [ctx.go:25-32](../internal/context/ctx.go#L25-L32) - NewManager, CreateContext
- [ctx.go:34-46](../internal/context/ctx.go#L34-L46) - CloneContext
- [ctx.go:48-66](../internal/context/ctx.go#L48-L66) - GetMessages, AddMessage, Clear
- [ctx.go:91-113](../internal/context/ctx.go#L91-L113) - Compress, ShouldCompress（Phase 2 新增）

## 与 Agent 的集成

Agent 持有 Manager 实例：
```go
type Agent struct {
    ctxManager *agentctx.Manager
}
```

每次对话使用独立的 Context：
```go
messageCtx, _ := ctxManager.CreateContext()
agent.RunStream(ctx, messageCtx, input, onToken)
```

## 性能考虑

- **内存占用**：每条消息 ~1KB，50 条 ~50KB
- **压缩开销**：O(n) 切片操作，n=30，可忽略
- **查找效率**：顺序遍历，O(n)，n 较小无影响

## 测试场景

1. **正常对话**：消息数 < 50，不触发压缩
2. **长对话**：消息数 > 50，自动压缩到 30 条
3. **压缩后继续**：压缩后可继续添加消息
4. **多轮压缩**：可多次触发压缩
