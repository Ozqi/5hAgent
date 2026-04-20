# Agent - 核心模块

## 位置
`internal/agent/agent.go` (~686行)

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
   - 调用LLM生成响应
   - 检查ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回响应

### RunStream(ctx, messageCtx, input, onToken) - 流式
流式ReAct循环：
1. 注入SystemPrompt(首次)
2. 添加用户消息
3. **上下文压缩检查**
   - 超过50条消息时自动压缩
   - 保留最近30条
4. 循环(最多MaxTurns):
   - 流式调用LLM
   - 读取chunks并调用onToken回调
   - **ToolCall合并**（避免多工具调用时arguments错误合并）
   - 收集完整响应和ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回完整内容

### exeTools(ctx, messageCtx, toolCalls) - 支持并发
执行工具列表（智能分类）：
1. **分类工具**
   - 只读工具: read_file, glob, grep, list_dir, task_get, task_list → 并发执行
   - 写工具: write_file, edit, exec_shell, task_create, task_update, task_delete → 串行执行
2. 并发执行只读工具: `exeToolsConcurrent()`
3. 串行执行写工具
4. 添加结果到messageCtx

### exeToolsConcurrent(ctx, messageCtx, toolCalls)
并发执行只读工具：
1. 创建结果channel
2. 为每个工具启动goroutine
3. 收集结果并按原顺序添加到上下文

## 关键特性

### 流式输出与ToolCall合并
使用列表+ID索引追踪工具调用，避免多工具调用时arguments错误合并

### 上下文压缩
自动检查消息数量，超过阈值时压缩历史消息

### 工具并发执行
只读工具自动并发执行，提升性能

## 相关文档
- 工具系统: `doc/tools.md`
- 上下文管理: `doc/context.md`
- 流式输出: `doc/stage2_streaming.md`
