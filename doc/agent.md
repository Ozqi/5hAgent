# Agent - 核心模块

## 位置
`internal/agent/agent.go` (~650行)

## 结构
```go
type Agent struct {
    model      model.ToolCallingChatModel  // LLM
    tools      []tool.BaseTool             // 工具列表
    toolMap    map[string]tool.BaseTool    // O(1)查找
    config     *Config                     // 配置
    state      *State                      // 状态
    ctxManager *agentctx.Manager           // 上下文管理
}
```

## 核心函数

### NewAgent(model, tools, config)
创建Agent，构建toolMap

### Run(ctx, messageCtx, input) - 非流式
ReAct循环：
1. 注入SystemPrompt(首次)
2. 添加用户消息
3. 循环(最多MaxTurns):
   - `resp, err := a.model.Generate(ctx, messages)` - 调用LLM
   - 检查ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回响应

### RunStream(ctx, messageCtx, input, onToken) - 流式 ⭐
流式ReAct循环：
1. 注入SystemPrompt(首次)
2. 添加用户消息
3. **上下文压缩检查** (agent.go:236-243)
   - `ShouldCompress()` 检查是否超过50条消息
   - `Compress()` 保留最近30条
4. 循环(最多MaxTurns):
   - `reader, err := a.model.Stream(ctx, messages)` - 流式调用
   - 读取chunks: `chunk, err := reader.Recv()`
   - **ToolCall合并** (agent.go:271-332)
     - 使用列表+ID索引追踪工具调用
     - 避免多工具调用时arguments错误合并
   - 每个chunk调用onToken回调
   - 收集完整响应和ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回完整内容

### exeTools(ctx, messageCtx, toolCalls) - 支持并发 ⭐
执行工具列表（智能分类）：
1. **分类工具** (agent.go:428-445)
   - 只读工具: read_file, glob → 并发执行
   - 写工具: edit, exec_shell → 串行执行
2. 并发执行只读工具: `exeToolsConcurrent()`
3. 串行执行写工具
4. 添加结果到messageCtx

### exeToolsConcurrent(ctx, messageCtx, toolCalls) - 新增 ⭐
并发执行只读工具：
1. 创建结果channel: `results := make(chan toolResult, len(toolCalls))`
2. 为每个工具启动goroutine
3. 收集结果并按原顺序添加到上下文

## 关键代码

### 流式输出与ToolCall合并
- `agent.go:265` - LLM Stream: `reader, err := a.model.Stream(ctx, messages)`
- `agent.go:271-332` - ToolCall合并逻辑:
  ```go
  var toolCallsList []*schema.ToolCall
  toolCallsIndex := make(map[string]int)
  
  if tc.ID != "" {
      // 新工具调用
      toolCallsList = append(toolCallsList, &tcCopy)
      toolCallsIndex[tc.ID] = len(toolCallsList) - 1
  } else {
      // 合并到最后一个工具
      lastTC := toolCallsList[len(toolCallsList)-1]
      lastTC.Function.Arguments += tc.Function.Arguments
  }
  ```

### 上下文压缩
- `agent.go:236-243` - 压缩检查:
  ```go
  if a.ctxManager.ShouldCompress(messageCtx) {
      before, after, err := a.ctxManager.Compress(messageCtx)
      logger.InfoTag("CTX", "Context compressed: %d -> %d messages", before, after)
  }
  ```

### 工具并发执行
- `agent.go:428-445` - 工具分类
- `agent.go:550-630` - 并发执行实现:
  ```go
  for idx, tc := range toolCalls {
      go func(idx int, tc schema.ToolCall) {
          // 执行工具
          results <- toolResult{idx: idx, tc: tc, result: result, err: execErr}
      }(idx, tc)
  }
  ```

## 日志集成
- `agent.go:241` - 上下文压缩日志
- `agent.go:296-299` - ToolCall chunk详情
- `agent.go:355-358` - 合并后的ToolCall
- `agent.go:437` - 并发工具执行标记
