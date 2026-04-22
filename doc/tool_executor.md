# Tool Executor - 工具执行系统

## 位置

`internal/agent/tool_executor.go` (~312行)

## 概述

工具执行系统负责调度和执行 LLM 返回的工具调用，支持并发执行（只读工具）和串行执行（写工具），并自动适配两种工具接口。

## 核心设计

### 1. 双轨执行策略

```
ToolCalls
    ↓
分类（只读 vs 写入）
    ↓
┌─────────────┬─────────────┐
│  只读工具    │   写工具     │
│  并发执行    │   串行执行   │
└─────────────┴─────────────┘
    ↓              ↓
  结果收集      逐个执行
    ↓              ↓
    └──────┬───────┘
           ↓
    按顺序添加到上下文
```

**只读工具**（可并发）：
- `read_file`, `glob`, `grep`, `list_dir`
- `task_get`, `task_list`

**写工具**（必须串行）：
- `write_file`, `edit`, `exec_shell`
- `task_create`, `task_update`, `task_delete`

### 2. 接口适配机制

Eino 框架提供两种工具接口：

```go
// 旧接口：返回简单字符串
type InvokableTool interface {
    InvokableRun(ctx, argumentsInJSON string) (string, error)
}

// 新接口：返回富媒体结果
type EnhancedInvokableTool interface {
    InvokableRun(ctx, argument *schema.ToolArgument) (*schema.ToolResult, error)
}
```

**适配策略**：
1. 优先尝试 `EnhancedInvokableTool`（支持图片、音频等）
2. 降级到 `InvokableTool`（仅文本）
3. 都不支持则报错

## 核心函数

### exeTools(ctx, messageCtx, toolCalls)

**入口函数**，负责工具调用的分类和调度。

**代码链接**: [tool_executor.go:28-68](../internal/agent/tool_executor.go#L28-L68)

**流程**:
1. 遍历 `toolCalls`，根据工具名分类到 `readOnlyCalls` 或 `writeCalls`
2. 调用 `exeToolsConcurrent()` 并发执行只读工具
3. 调用 `executeSingleTool()` 逐个串行执行写工具

**示例**:
```go
toolCalls := []schema.ToolCall{
    {Function: {Name: "read_file", Arguments: "..."}},  // 只读
    {Function: {Name: "glob", Arguments: "..."}},       // 只读
    {Function: {Name: "write_file", Arguments: "..."}}, // 写入
}

// 执行结果：
// 1. read_file 和 glob 并发执行
// 2. write_file 串行执行
```

---

### exeToolsConcurrent(ctx, messageCtx, toolCalls)

**并发执行**只读工具，使用 goroutine + channel 模式。

**代码链接**: [tool_executor.go:70-122](../internal/agent/tool_executor.go#L70-L122)

**并发模型**:
```go
type toolResult struct {
    idx    int              // 原始索引（用于排序）
    tc     schema.ToolCall  // 工具调用信息
    result string           // 执行结果
    err    error            // 执行错误
}

results := make(chan toolResult, len(toolCalls))
```

**流程**:
1. 创建 buffered channel（容量 = 工具数量）
2. 为每个工具启动 goroutine 并发执行
3. 从 channel 收集结果
4. **按原始索引排序**（保证上下文顺序）
5. 逐个添加到上下文

**关键点**:
- 保序：虽然并发执行，但结果按原始顺序添加
- 错误隔离：一个工具失败不影响其他工具
- 性能提升：3 个工具并发，总耗时 = max(t1, t2, t3)

---

### executeSingleTool(ctx, messageCtx, tc, idx, total, concurrent)

**串行执行**单个工具。

**代码链接**: [tool_executor.go:147-179](../internal/agent/tool_executor.go#L147-L179)

**流程**:
1. 验证工具调用有效性（`tc.Function.Name != ""`）
2. 调用 `findTool()` 查找工具实例
3. 调用 `invokeTool()` 执行工具
4. 调用 `addToolResultToContext()` 添加结果

**参数**:
- `idx`, `total`: 用于日志显示进度（如 `[1/3]`）
- `concurrent`: 是否并发执行（影响日志显示）

---

### invokeTool(ctx, t, tc) ⭐ 核心

**工具调用核心逻辑**，处理两种接口的适配。

**代码链接**: [tool_executor.go:181-207](../internal/agent/tool_executor.go#L181-L207)

**执行流程**:

```go
// 1. 尝试 EnhancedInvokableTool (新接口)
if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
    toolArg := &schema.ToolArgument{
        Text: tc.Function.Arguments,  // LLM 返回的 JSON 参数
    }
    toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
    if err != nil {
        return "", err
    }
    // 将 ToolResult 转换为字符串
    return formatToolResult(toolResult), nil
}

// 2. 降级到 InvokableTool (旧接口)
if invokable, ok := t.(tool.InvokableTool); ok {
    return invokable.InvokableRun(ctx, tc.Function.Arguments)
}

// 3. 都不支持则报错
return "", fmt.Errorf("tool %s is not invokable", tc.Function.Name)
```

**为什么这样设计**:
- ✅ 向后兼容：旧工具无需修改
- ✅ 渐进增强：新工具可以返回富媒体
- ✅ 统一逻辑：避免重复代码

**类型断言语法**:
```go
if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
    // t 实现了 EnhancedInvokableTool 接口
    // enhancedInvokable 是转换后的类型
}
```

---

### addToolResultToContext(messageCtx, tc, result, execErr)

**统一处理**工具执行结果，添加到上下文。

**代码链接**: [tool_executor.go:209-232](../internal/agent/tool_executor.go#L209-L232)

**处理逻辑**:

```go
if execErr != nil {
    // 失败：添加错误消息
    errMsg := schema.ToolMessage(
        fmt.Sprintf("tool execution failed: %v", execErr),
        tc.ID,
    )
    return a.ctxManager.AddMessage(messageCtx, errMsg)
}

// 成功：添加结果
resultMsg := schema.ToolMessage(result, tc.ID)
return a.ctxManager.AddMessage(messageCtx, resultMsg)
```

**优势**: 避免在串行执行和并发执行中重复结果处理代码

---

### formatToolResult(toolResult)

**格式化** `EnhancedInvokableTool` 返回的 `*schema.ToolResult`。

**代码链接**: [tool_executor.go:234-256](../internal/agent/tool_executor.go#L234-L256)

**ToolResult 结构**:
```go
type ToolResult struct {
    Parts []ToolOutputPart  // 可以包含多个部分
}

type ToolOutputPart struct {
    Type string  // text, image, audio, video, file
    Text string  // 文本内容
    // ... 其他字段
}
```

**转换逻辑**:
```go
var parts []string
for _, part := range toolResult.Parts {
    switch part.Type {
    case schema.ToolPartTypeText:
        parts = append(parts, part.Text)
    case schema.ToolPartTypeImage:
        parts = append(parts, "[Image]")  // 图片占位符
    case schema.ToolPartTypeAudio:
        parts = append(parts, "[Audio]")
    // ... 其他类型
    }
}
return strings.Join(parts, "\n")
```

---

### executeToolStreaming(ctx, messageCtx, toolCall)

**流式输出中的异步工具执行**。

**代码链接**: [tool_executor.go:124-145](../internal/agent/tool_executor.go#L124-L145)

**使用场景**: 在流式输出过程中，检测到完整的工具调用时立即异步执行

**特点**:
- 在 goroutine 中调用，不阻塞流式输出
- 错误不返回，只记录到日志
- 用于"边输出边执行"的优化

---

### findTool(name)

**查找工具**实例。

**代码链接**: [tool_executor.go:258-265](../internal/agent/tool_executor.go#L258-L265)

**实现**:
```go
func (a *Agent) findTool(name string) tool.BaseTool {
    return a.toolMap[name]  // O(1) 查找
}
```

---

### isValidJSON(s)

**验证 JSON** 字符串有效性。

**代码链接**: [tool_executor.go:267-271](../internal/agent/tool_executor.go#L267-L271)

**实现**:
```go
func isValidJSON(s string) bool {
    var js json.RawMessage
    return json.Unmarshal([]byte(s), &js) == nil
}
```

**用途**: 在流式输出中判断工具参数是否完整

## 执行流程示例

### 示例 1：混合工具调用

```
用户: 读取 task.md 和 README.md，然后创建一个新文件 summary.md

LLM 返回:
[
  {id: "1", name: "read_file", args: '{"path": "task.md"}'},
  {id: "2", name: "read_file", args: '{"path": "README.md"}'},
  {id: "3", name: "write_file", args: '{"path": "summary.md", "content": "..."}'}
]

执行流程:
1. exeTools() 分类:
   - readOnlyCalls: [1, 2]
   - writeCalls: [3]

2. exeToolsConcurrent([1, 2]):
   - 并发执行 read_file(task.md) 和 read_file(README.md)
   - 收集结果，按顺序 [1, 2] 添加到上下文

3. executeSingleTool(3):
   - 串行执行 write_file(summary.md)
   - 添加结果到上下文

结果: 两个读取并发执行（快），写入串行执行（安全）
```

### 示例 2：接口适配

```
工具: read_file (实现了 EnhancedInvokableTool)

调用流程:
1. invokeTool(ctx, read_file, tc)
2. 类型断言: t.(tool.EnhancedInvokableTool) ✅
3. 调用: toolResult, err := t.InvokableRun(ctx, toolArg)
4. 格式化: result = formatToolResult(toolResult)
5. 返回: result = '{"content": "...", "total_lines": 100}'
```

## 设计优势

### 1. 性能优化

- **并发执行只读工具**: 3 个文件读取从 3s 降到 1s
- **串行执行写工具**: 避免文件冲突和数据竞争

### 2. 代码复用

- `invokeTool()`: 统一接口适配逻辑
- `addToolResultToContext()`: 统一结果处理
- `executeSingleTool()`: 统一单工具执行流程

### 3. 职责分离

- `agent.go`: 专注 ReAct 循环和流式输出
- `tool_executor.go`: 专注工具执行和调度

### 4. 扩展性

- 新增只读工具：只需在 `readOnlyTools` map 中添加
- 新增工具接口：在 `invokeTool()` 中添加新的类型断言

## 并发安全

### 只读工具并发安全的原因

- 不修改共享状态
- 不写入文件系统
- 不修改数据库

### 写工具必须串行的原因

- 避免同时写入同一文件
- 避免数据竞争（如任务列表）
- 保证操作顺序（如先创建目录再写文件）

## 错误处理

### 工具未找到

```go
if t == nil {
    errMsg := schema.ToolMessage("tool not found: xxx", tc.ID)
    a.ctxManager.AddMessage(messageCtx, errMsg)
    continue  // 继续执行其他工具
}
```

### 工具执行失败

```go
if execErr != nil {
    errMsg := schema.ToolMessage("tool execution failed: ...", tc.ID)
    a.ctxManager.AddMessage(messageCtx, errMsg)
    continue  // 继续执行其他工具
}
```

### 工具不支持任何接口

```go
return "", fmt.Errorf("tool %s is not invokable", tc.Function.Name)
```

**设计原则**: 一个工具失败不影响其他工具执行

## 相关文档

- [Agent 核心](agent.md) - ReAct 循环和流式输出
- [Tools 工具系统](tools.md) - 工具实现和注册
- [Context 上下文管理](context.md) - 消息存储和压缩

## 相关文件

- `internal/agent/tool_executor.go` - 工具执行逻辑
- `internal/agent/agent.go` - Agent 核心（调用 exeTools）
- `internal/tools/*.go` - 工具实现
