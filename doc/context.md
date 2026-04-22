# Context - 上下文管理

```text
internal/context/ctx.go
  -> Context.messages
  -> Manager.CreateContext()
  -> Manager.AddMessage()
  -> Manager.Compress()
```

## 位置

- `internal/context/ctx.go`

## 概述

当前上下文实现非常轻量：

- `Context` 只保存消息切片
- `Manager` 本身无状态
- 压缩策略是简单截断，不做摘要

## 结构

```go
type Context struct {
    messages []*schema.Message
}

type Manager struct{}
```

## 常量

- `MaxMessages = 50`
- `KeepRecentMessages = 30`

含义：

- 消息数大于 50 时允许压缩
- 压缩后只保留最近 30 条

## Manager 接口

### `NewManager()`

创建无状态 manager。

### `CreateContext()`

创建空上下文。

### `CloneContext(parent)`

复制父上下文的消息切片，用于创建隔离副本。

### `GetMessages(ctx)`

返回当前全部消息。

### `AddMessage(ctx, msg)`

把消息追加到末尾。

### `Clear(ctx)`

清空消息列表。

### `ShouldCompress(ctx)`

检查 `len(messages) > MaxMessages`。

### `Compress(ctx)`

如果消息过多，则截断为最近 `KeepRecentMessages` 条。

## 当前压缩策略

实现非常直接：

```go
if beforeCount <= MaxMessages {
    return beforeCount, beforeCount, nil
}

keepStart := beforeCount - KeepRecentMessages
ctx.messages = ctx.messages[keepStart:]
```

特点：

- 简单
- 快
- 会丢失早期上下文

## 与 Agent 的集成

`Agent.RunStream()` 会在添加用户消息后检查是否需要压缩。

如果触发压缩：

1. 调用 `ShouldCompress()`
2. 调用 `Compress()`
3. 记录日志
4. 在终端打印压缩提示

当前 `Run()` 非流式路径没有单独压缩步骤。

## 当前实现边界

- 不区分 system / user / assistant / tool 消息的重要性。
- 不保留摘要消息。
- 不做持久化分层存储。
- 不做基于 token 或工具输出大小的压缩。

## 相关代码

- [ctx.go](../internal/context/ctx.go)
- [agent.go](../internal/agent/agent.go)
