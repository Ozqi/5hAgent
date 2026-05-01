# 重构计划 - 使用 Eino 框架能力

## 重构目标

减少重复代码，利用 Eino 原生能力替代手写实现。

## 当前代码痛点

| 文件 | 重复代码 | 行数 | Eino 等价能力 |
|------|----------|------|--------------|
| `tool_use.go` | `streamToolCollector` | ~120 | `schema.ToolCall.Index` |
| `tool_use.go` | `exeTools/exeToolsPar` | ~70 | `ToolsNode` |
| `tool_use.go` | `isReadOnly` | ~20 | `ToolMiddleware` |
| `agent.go` | `toolRepeatGuard` | ~30 | `ToolMiddleware` |
| `agent.go` | `mergeMeta` | ~40 | `Callback` |
| 全局 | `logger.DebugTag` | 分散 | `Callback` |

---

## 重构阶段

### Phase 1: Eino Callback 日志系统（低风险）

**目标**: 用 Eino 原生 Callback 替代手写日志

**改动**:
- 创建 `internal/agent/callbacks.go`
- 注册 `ModelCallbackHandler` 和 `ToolCallbackHandler`
- 移除分散的 `logger.DebugTag` 调用

**预期效果**:
- 统一的日志和监控
- 更好的可观测性

```go
// internal/agent/callbacks.go (新增)
type AgentCallbacks struct {
    debug      bool
    tokenUsage *int
}

func NewAgentCallbacks(debug bool) *AgentCallbacks {
    return &AgentCallbacks{debug: debug, tokenUsage: new(int)}
}

// 模型开始调用
func (c *AgentCallbacks) OnModelStart(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
    if c.debug {
        log.Printf("[LLM] Start: %d messages", len(input.Messages))
    }
    return ctx
}

// 模型调用结束
func (c *AgentCallbacks) OnModelEnd(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
    if c.debug && output.TokenUsage != nil {
        *c.tokenUsage += output.TokenUsage.TotalTokens
        log.Printf("[LLM] Tokens: %d (total: %d)", output.TokenUsage.TotalTokens, *c.tokenUsage)
    }
    return ctx
}

// 工具开始调用
func (c *AgentCallbacks) OnToolStart(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
    if c.debug {
        log.Printf("[TOOL] Start: %s", info.Name)
    }
    return ctx
}

// 工具调用结束
func (c *AgentCallbacks) OnToolEnd(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
    if c.debug {
        log.Printf("[TOOL] End: %s", info.Name)
    }
    return ctx
}
```

---

### Phase 2: 简化 streamToolCollector（中风险）

**目标**: 利用 Eino 的 `Index` 字段简化合并逻辑

**现状分析**:
- 当前: ID-based 手动合并（复杂）
- Eino: `ToolCall.Index` 字段支持流式合并

**改动**:
- 修改 `streamToolCollector.Add()` 使用 Index 字段
- 简化 merge 逻辑

```go
// 简化版 - 使用 Index 字段
func (c *streamToolCollector) Add(chunks []schema.ToolCall) []schema.ToolCall {
    if len(chunks) == 0 {
        return nil
    }

    for _, tc := range chunks {
        idx := 0
        if tc.Index != nil {
            idx = *tc.Index
        }

        // 扩展 states 数组
        for len(c.states) <= idx {
            c.states = append(c.states, &toolState{})
        }

        // 合并到指定位置
        c.mergeIndex(idx, tc)
    }

    return c.extractReady()
}

// 简化合并逻辑
func (c *streamToolCollector) mergeIndex(idx int, tc schema.ToolCall) {
    state := c.states[idx]
    if state.call.ID == "" && tc.ID != "" {
        state.call.ID = tc.ID
    }
    if state.call.Function.Name == "" && tc.Function.Name != "" {
        state.call.Function.Name = tc.Function.Name
    }
    if tc.Function.Arguments != "" {
        state.call.Function.Arguments += tc.Function.Arguments
    }
}
```

---

### Phase 3: ToolsNode 替代手写调度（高风险）

**目标**: 用 Eino `ToolsNode` 替代 `exeTools/exeToolsPar`

**挑战**:
- 当前逻辑: 只读工具并发，写工具串行
- ToolsNode 默认并发，可配置 `ExecuteSequentially`

**方案**: 创建两个 ToolsNode

```go
// internal/agent/tool_node.go (新增)

// ToolExecutors 工具执行器
type ToolExecutors struct {
    readOnly *compose.ToolsNode  // 只读: 并发
    writable *compose.ToolsNode  // 写: 串行
}

// NewToolExecutors 创建执行器
func NewToolExecutors(tools []tool.BaseTool) (*ToolExecutors, error) {
    readOnly, writable := classifyTools(tools)
    
    readNode, err := compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
        Tools: readOnly,
        ExecuteSequentially: false,  // 并发
        ToolCallMiddlewares: []compose.ToolMiddleware{repeatGuardMiddleware()},
    })
    if err != nil {
        return nil, err
    }

    writeNode, err := compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
        Tools: writable,
        ExecuteSequentially: true,  // 串行
        ToolCallMiddlewares: []compose.ToolMiddleware{repeatGuardMiddleware()},
    })
    if err != nil {
        return nil, err
    }

    return &ToolExecutors{readOnly: readNode, writable: writeNode}, nil
}

// Execute 并发执行只读，串行执行写操作
func (e *ToolExecutors) Execute(ctx context.Context, calls []schema.ToolCall) ([]*schema.Message, error) {
    // 分类
    readCalls, writeCalls := classifyCalls(calls)
    
    var results []*schema.Message
    
    // 并发执行只读
    if len(readCalls) > 0 {
        msgs, err := e.readOnly.Invoke(ctx, &schema.Message{
            Role: schema.Assistant,
            ToolCalls: readCalls,
        })
        if err != nil {
            return nil, err
        }
        results = append(results, msgs...)
    }
    
    // 串行执行写
    if len(writeCalls) > 0 {
        msgs, err := e.writable.Invoke(ctx, &schema.Message{
            Role: schema.Assistant,
            ToolCalls: writeCalls,
        })
        if err != nil {
            return nil, err
        }
        results = append(results, msgs...)
    }
    
    return results, nil
}
```

---

### Phase 4: ToolMiddleware 防护（中风险）

**目标**: 用 `ToolMiddleware` 实现重复防护

```go
// repeatGuardMiddleware 创建重复防护中间件
func repeatGuardMiddleware(limit int) compose.ToolMiddleware {
    attempts := make(map[string]int)
    
    return compose.ToolMiddleware{
        EnhancedInvokable: func(next compose.EnhancedInvokableToolEndpoint) compose.EnhancedInvokableToolEndpoint {
            return func(ctx context.Context, input *compose.ToolInput) (*compose.EnhancedInvokableToolOutput, error) {
                name := input.Name
                args := input.Arguments
                
                // 生成唯一 key
                key := name + ":" + normalizeArgs(args)
                
                attempts[key]++
                if attempts[key] > limit {
                    return nil, fmt.Errorf("repeated tool call detected: %s", name)
                }
                
                return next(ctx, input)
            }
        },
    }
}
```

---

## 执行顺序

```
Phase 1 ──┬── Phase 2 ─── Phase 3 ─── Phase 4
          │     │            │            │
          │     └──────┬─────┴────────────┘
          │            │
          └────────────┴── 独立，可并行
```

| 阶段 | 风险 | 工作量 | 依赖 |
|------|------|--------|------|
| Phase 1 | 低 | 小 | 无 |
| Phase 2 | 中 | 中 | Phase 1 |
| Phase 3 | 高 | 大 | Phase 2 |
| Phase 4 | 中 | 中 | Phase 3 |

---

## 保留的功能（不重构）

| 功能 | 原因 |
|------|------|
| TaskList 持久化 | 业务逻辑 |
| Skill 注入 | 业务逻辑 |
| MCP 客户端 | 特定协议 |
| TUI 界面 | 业务逻辑 |
| LMCompress | 业务逻辑 |

---

## 预期收益

| 指标 | 现状 | 重构后 |
|------|------|--------|
| 代码行数 (tool_use.go) | ~400 | ~280 |
| 代码行数 (agent.go) | ~480 | ~400 |
| 测试覆盖 | 手动 | 可用 Eino 测试 |
| 可观测性 | 有限 | Callback 完整 |

---

## 当前进度

### ✅ Phase 1: Callback 日志系统 (已完成)
- 新增 `callbacks.go`: AgentCallbacks 处理器
- 模型/工具调用日志统一管理
- Token 统计

### ✅ Phase 2: 简化 streamToolCollector (已完成)
- 使用 `map[int]` 替代 slice + ID 映射
- 利用 Index 字段简化合并逻辑
- 代码从 141 行减少到 93 行

### ✅ Phase 3: 删除未使用代码 (已完成)
- 移除 `exeTools()` (未被调用)
- 移除 `exeToolsPar()` (未被调用)
- 移除 `isReadOnly()` (未被调用)
- tool_use.go 从 408 行减少到 280 行

### ⏸️ Phase 4: ToolsNode 架构 (待定)
- 当前流式执行模式与 ToolsNode 批量执行模式不同
- 需要较大架构变更
- 建议: 保持当前实现，未来按需迁移

---

## Git 提交记录

```bash
# Phase 1: Callback 系统
git commit -m "refactor(agent): Phase 1 - 引入 Eino Callback 系统"

# Phase 2-3: 简化工具调用代码
git commit -m "refactor(agent): Phase 2-3 简化工具调用代码"
```
