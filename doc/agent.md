# Agent - 核心模块

## 位置
`internal/agent/agent.go` (~430行)

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

### RunStream(ctx, messageCtx, input, onToken) - 流式
流式ReAct循环：
1. 注入SystemPrompt(首次)
2. 添加用户消息
3. 循环(最多MaxTurns):
   - `reader, err := a.model.Stream(ctx, messages)` - 流式调用
   - 读取chunks: `chunk, err := reader.Recv()`
   - 每个chunk调用onToken回调
   - 收集完整响应和ToolCalls
   - 有工具 → exeTools() → 继续
   - 无工具 → 返回完整内容

### exeTools(ctx, messageCtx, toolCalls)
执行工具列表：
1. 遍历toolCalls
2. findTool(name) - O(1)查找
3. `result, err := invokable.InvokableRun(ctx, args)` - 执行
4. 添加结果到messageCtx

## 关键代码
- `agent.go:152` - LLM Generate: `resp, err := a.model.Generate(ctx, messages)`
- `agent.go:250` - LLM Stream: `reader, err := a.model.Stream(ctx, messages)`
- `agent.go:265` - 读取chunk: `chunk, err := reader.Recv()`
- `agent.go:412` - 工具执行: `result, err := invokable.InvokableRun(ctx, tc.Function.Arguments)`

## 日志集成
- `agent.go:137` - ReAct轮次
- `agent.go:144-148` - 消息详情(role对齐，内容摘要)
- `agent.go:151-157` - LLM调用和响应
- `agent.go:161-166` - 工具调用请求
- `agent.go:378-390` - 工具执行流程
