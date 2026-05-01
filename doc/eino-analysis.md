# Eino 框架能力分析 - 避免重复造轮子

## 1. Eino 框架概述

Eino 是字节跳动 CloudWeGo 团队开源的 Go LLM 应用开发框架，提供：

- **组件层 (Components)**：ChatModel、Tool、Retriever、Embedding 等抽象接口
- **编排层 (Compose)**：Chain、Graph、Workflow 三大编排引擎
- **Agent 开发套件 (ADK)**：预置 Agent 范式、多智能体编排
- **工具链**：Callback、State、Option 等横切能力

```mermaid
┌─────────────────────────────────────────────────────────────┐
│  应用层（Flow/ADK）：预置 Agent 范式、多智能体编排            │
├─────────────────────────────────────────────────────────────┤
│  编排层（Compose）：Chain/Graph/Workflow 三大编排引擎         │
├─────────────────────────────────────────────────────────────┤
│  组件层（Components）：LLM 应用原子组件接口抽象               │
├─────────────────────────────────────────────────────────────┤
│  基础层（Schema）：统一数据模型、通用契约、流处理基础能力       │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. Eino 已有的能力 vs 5hAgent 重复实现

### 2.1 Tool 接口与工具创建

| 功能 | Eino 提供 | 5hAgent 现状 | 建议 |
|------|----------|-------------|------|
| 基础工具接口 | `InvokableTool`、`StreamableTool` | 已正确使用 | ✅ 复用 |
| 增强工具接口 | `EnhancedInvokableTool`（支持多模态） | `utils.InferEnhancedTool` | ✅ 复用 |
| 工具信息注册 | `schema.ToolInfo`、`schema.NewParamsOneOfByParams` | 手动拼装 | ⚠️ 可优化 |
| 工具自动绑定 | `model.WithTools()` | 手动传递 | ✅ 复用 |

**当前 5hAgent 正确复用的部分**：

```go
// tools/grep.go - 正确使用 InferEnhancedTool
func NewGrepTool() (tool.EnhancedInvokableTool, error) {
    return utils.InferEnhancedTool(
        "base.grep",
        "Search for patterns...",
        func(ctx context.Context, input GrepInput) (*schema.ToolResult, error) {
            // 业务逻辑
        },
    )
}
```

### 2.2 ToolsNode - 工具执行器

Eino 的 `compose.NewToolsNode` 提供了完整的工具执行框架：

```go
// Eino ToolsNode 配置
toolsNode, err := compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
    Tools: []tool.BaseTool{searchTool, weatherTool},
    
    // 串行执行（默认并发）
    ExecuteSequentially: false,
    
    // 未知工具处理
    UnknownToolsHandler: func(ctx context.Context, name, input string) (string, error) {
        return "", fmt.Errorf("tool not found: %s", name)
    },
    
    // 工具参数预处理
    ToolArgumentsHandler: func(ctx context.Context, name, arguments string) (string, error) {
        return arguments, nil
    },
    
    // 中间件
    ToolCallMiddlewares: []compose.ToolMiddleware{{
        EnhancedInvokable: func(next compose.EnhancedInvokableToolEndpoint) compose.EnhancedInvokableToolEndpoint {
            return func(ctx context.Context, input *compose.ToolInput) (*compose.EnhancedInvokableToolOutput, error) {
                // 前置处理
                log.Printf("Calling tool: %s", input.Name)
                output, err := next(ctx, input)
                // 后置处理
                return output, err
            }
        },
    }},
})
```

**当前 5hAgent 重复实现的部分**：

| 5hAgent 重复实现 | Eino 等价能力 |
|------------------|-------------|
| `streamToolCollector` 合并 ToolCall | `schema.ToolCall.Index` 字段已支持 |
| `exeTools` / `exeToolsPar` 并发调度 | `ToolsNode` 的并发执行 |
| `isReadOnly` 只读判断 | 可用 `ToolMiddleware` 实现 |
| `invokeTool` 接口适配 | `ToolsNode` 自动处理 |
| `toolRepeatGuard` 重复防护 | 可用 `ToolMiddleware` 实现 |

### 2.3 流式 ToolCall 合并

**当前 5hAgent 实现** (`tool_use.go:27-84`)：

```go
type streamToolCollector struct {
    states []*toolState
    byID   map[string]int
}

func (c *streamToolCollector) Add(chunks []schema.ToolCall) []schema.ToolCall {
    // 手动合并分片
    for _, tc := range chunks {
        c.merge(tc)
    }
    // 检查是否完整
}
```

**Eino 原生支持**：

Eino 的 `schema.Message.ToolCalls` 已包含 `Index` 字段：

```go
// schema/message.go
type ToolCall struct {
    // Index 用于流式模式下合并 ToolCall 分片
    Index *int `json:"index,omitempty"`
    ID    string `json:"id"`
    // ...
}
```

`ToolsNode` 会自动处理分片合并，无需手动实现 `streamToolCollector`。

### 2.4 Callback 机制

Eino 提供了完整的 Callback 系统，可用于日志、监控、调试：

```go
import callbacksHelper "github.com/cloudwego/eino/utils/callbacks"

// 创建工具回调处理器
toolHandler := &callbacksHelper.ToolCallbackHandler{
    OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
        log.Printf("Starting tool: %s, args: %s", info.Name, input.ArgumentsInJSON)
        return ctx
    },
    OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
        log.Printf("Tool completed: %s", info.Name)
        return ctx
    },
    OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
        log.Printf("Tool error: %s, err: %v", info.Name, err)
        return ctx
    },
}

// 创建模型回调处理器
modelHandler := &callbacksHelper.ModelCallbackHandler{
    OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
        log.Printf("LLM request: %d messages", len(input.Messages))
        return ctx
    },
    OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
        log.Printf("LLM response: tokens=%d", output.TokenUsage.TotalTokens)
        return ctx
    },
}

// 组合使用
helper := callbacksHelper.NewHandlerHelper().
    ChatModel(modelHandler).
    Tool(toolHandler).
    Handler()

// 应用到 Chain/Graph
runnable, _ := chain.Compile()
result, err := runnable.Invoke(ctx, input, compose.WithCallbacks(helper))
```

**当前 5hAgent 重复实现**：

| 5hAgent 重复实现 | Eino 等价能力 |
|------------------|-------------|
| `mergeMeta` 合并 token 统计 | `model.CallbackOutput.TokenUsage` |
| `logger.DebugTag` 调试输出 | `Callback` 系统 |
| `toolRepeatGuard` 重复防护 | `ToolMiddleware` |

---

## 3. Eino ADK - Agent 开发套件

### 3.1 Agent 接口

Eino ADK 提供了标准化的 Agent 接口：

```go
type Agent interface {
    Name(ctx context.Context) string
    Description(ctx context.Context) string
    Run(ctx context.Context, input *AgentInput) *AsyncIterator[*AgentEvent]
}

type AgentInput struct {
    Messages    []*schema.Message  // 对话历史
    Streaming   bool               // 是否流式
    UserInput   string             // 用户输入
}

type AgentEvent struct {
    Type    EventType
    Message *schema.Message
    ToolCall *schema.ToolCall
    Error   error
}
```

### 3.2 预置 Agent 模式

| 模式 | 说明 | 适用场景 |
|------|------|---------|
| `ChatModelAgent` | 基于 ChatModel 的基础 Agent | 简单对话 |
| `SequentialAgent` | 顺序执行多个 Agent | 流水线处理 |
| `ParallelAgent` | 并发执行多个 Agent | 并行查询 |
| `LoopAgent` | 循环执行直到满足条件 | 迭代优化 |
| `SupervisorAgent` | 主 Agent 控制子 Agent | 任务委派 |
| `PlanExecuteAgent` | 计划-执行-重规划循环 | 复杂任务 |

### 3.3 Runner 与检查点

```go
import "github.com/cloudwego/eino/agent"

runner := agent.NewRunner(agentConfig)

// 运行 Agent
err := runner.Run(ctx, agentInput)

// 支持中断和恢复
runner.Interrupt()  // 中断执行
runner.Resume(ctx)  // 恢复执行

// 检查点保存
checkpoint := runner.SaveCheckpoint()
runner.Restore(checkpoint)
```

---

## 4. 具体重构建议

### 4.1 移除 streamToolCollector，改用 Eino 流式处理

当前实现可以简化为依赖 Eino 的 `Index` 字段和 `ToolsNode`。

### 4.2 使用 ToolsNode 替代手写并发调度

```go
// 旧代码 (tool_use.go:151-182)
func (a *Agent) exeTools(ctx, messageCtx, toolCalls) error {
    readOnlyCalls, writeCalls := classify(toolCalls)
    // 手动并发/串行调度
}

// 新代码 - 使用 Eino ToolsNode
toolsNode := compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
    Tools: a.tools,
    ExecuteSequentially: false,  // 并发执行
    ToolCallMiddlewares: []compose.ToolMiddleware{{
        EnhancedInvokable: repeatGuardMiddleware(),
    }},
})
```

### 4.3 使用 Callback 替代手写日志

```go
// 旧代码
logger.DebugTag("LLM", "Calling Stream")

// 新代码
handler := &callbacksHelper.ModelCallbackHandler{
    OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
        logger.DebugTag("LLM", "Calling Stream with %d messages", len(input.Messages))
        return ctx
    },
}
```

### 4.4 tokenBudget 复用

当前 `utils/tokenBudget.go` 可考虑使用 Eino 的 `TokenUsage` 回调：

```go
modelHandler := &callbacksHelper.ModelCallbackHandler{
    OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
        if output.TokenUsage != nil {
            // 累计 token
            totalTokens.Add(output.TokenUsage.TotalTokens)
        }
        return ctx
    },
}
```

---

## 5. 不建议迁移的部分

以下功能是 5hAgent 的特色实现，Eino ADK 未直接提供：

| 功能 | 说明 | 原因 |
|------|------|------|
| TaskList 持久化 | 任务列表的 Markdown 存储 | 业务特定逻辑 |
| Skill 注入 | 动态加载和启用技能 | 业务特定逻辑 |
| MCP 集成 | MCP 协议客户端 | Eino MCP 是服务端 |
| TUI 界面 | Bubble Tea 终端界面 | 业务特定逻辑 |
| 上下文压缩 | LMCompress 摘要替换 | 需要配合业务逻辑 |

---

## 6. 迁移优先级

### 高优先级（减少重复代码）

1. **移除 streamToolCollector**
   - 使用 Eino 的 `schema.ToolCall.Index` 字段
   - 让 `ToolsNode` 处理合并

2. **使用 ToolsNode 替代并发调度**
   - 减少 `exeTools`、`exeToolsPar` 的手写代码
   - 使用 `ToolMiddleware` 实现重复防护

3. **使用 Callback 替代 Debug 日志**
   - 复用 Eino 的 `callbacksHelper`
   - 保持调试能力

### 中优先级（提升架构）

4. **考虑使用 Eino ADK Agent 接口**
   - 如果未来需要多 Agent 协作
   - 当前单 Agent 可保持现状

5. **迁移到 Eino Chain/Graph**
   - 如果需要更复杂的编排逻辑

### 低优先级（保持现状）

- TaskList、Skill、MCP、TUI 等业务逻辑
- 这些是 5hAgent 的特色，不在 Eino 范围内

---

## 7. 参考资料

- [Eino Overview](https://www.cloudwego.io/docs/eino/overview/)
- [Eino ADK](https://www.cloudwego.io/docs/eino/core_modules/eino_adk/)
- [ToolsNode Guide](https://www.cloudwego.io/docs/eino/core_modules/components/tools_node_guide/)
- [ChatModel Guide](https://www.cloudwego.io/docs/eino/core_modules/components/chat_model_guide/)
- [Eino Examples](https://github.com/cloudwego/eino-examples)
