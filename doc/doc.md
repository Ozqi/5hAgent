# miniAgent 架构文档

> 代码量：~1500行 | 技术栈：Go 1.23 + Eino + Claude API

**相关文档**:
- [Eino框架使用说明](./eino_usage.md)
- [Agent模块详解](./agent.md)
- [Logger日志模块](./logger.md)
- [Stage1代码Review](./stage1_review.md)
- [Stage2流式输出](./stage2_streaming.md)

---

## 核心模块

### 1. LLM 客户端 (`internal/llm/`)

**client.go** (107行)
- `NewClientFromEnv(ctx, envPath)`: 从.env加载配置创建客户端
- `NewClient(ctx, config)`: 创建LLM客户端，封装Eino的claude.NewChatModel
- `GetModel()`: 返回ToolCallingChatModel接口

**配置**: CLAUDE_API_KEY, CLAUDE_BASE_URL, CLAUDE_MODEL

---

### 2. Agent 核心 (`internal/agent/`)

**agent.go** (~430行)

**结构体**:
```go
type Agent struct {
    model      model.ToolCallingChatModel  // LLM模型
    tools      []tool.BaseTool             // 工具列表
    toolMap    map[string]tool.BaseTool    // 工具映射表(O(1)查找)
    config     *Config                     // 配置
    state      *State                      // 状态
    ctxManager *agentctx.Manager           // 上下文管理器(复用)
}
```

**关键函数**:
- `NewAgent(model, tools, config)`: 创建Agent实例，构建toolMap
- `Run(ctx, messageCtx, input)`: 非流式ReAct循环
  - 核心: `resp, err := a.model.Generate(ctx, messages)`
- `RunStream(ctx, messageCtx, input, onToken)`: 流式ReAct循环
  - 核心: `reader, err := a.model.Stream(ctx, messages)`
  - 读取: `chunk, err := reader.Recv()`
- `exeTools(ctx, messageCtx, toolCalls)`: 执行工具调用列表
  - 核心: `result, err := invokable.InvokableRun(ctx, args)`

---

### 3. 工具系统 (`internal/tools/`)

**registry.go**: 工具注册表
- `GetAllTools()`: 返回所有可用工具

**read_file.go**: 读取文件内容
**exec_shell.go**: 执行shell命令

**工具接口**: 实现Eino的`tool.BaseTool`和`tool.InvokableTool`

---

### 4. 上下文管理 (`internal/context/`)

**ctx.go**:
- `Manager`: 上下文管理器
  - `CreateContext()`: 创建新上下文
  - `AddMessage(ctx, msg)`: 添加消息
  - `GetMessages(ctx)`: 获取所有消息
- `Context`: 消息上下文（封装消息历史）

---

### 5. 日志系统 (`internal/logger/`)

**logger.go** (~130行): 核心日志
- 4级日志(DEBUG/INFO/WARN/ERROR)，标签分类，格式对齐
- `DebugTag(tag, format, args...)`: 带标签日志
- `TruncateString(s, maxLen)`: 字符串摘要

**color.go** (~120行): 颜色支持
- ANSI颜色，级别/标签不同颜色
- `Red/Green/Yellow/Blue/Cyan/Magenta/Gray/Bold`: 颜色函数

---

### 6. CLI (`internal/cli/` + `cmd/miniagent/`)

**ui.go**: CLI输出函数
- `PrintError(err)`: 打印错误
- `PrintAssistantChunk(text)`: 打印助手响应

**main.go** (~166行): 主入口
1. 加载.env配置
2. 创建LLM客户端
3. 注册工具 - `main.go:68-70` 工具注册日志
4. 创建Agent
5. 创建上下文管理器
6. 启动readline交互循环
7. 处理用户输入 → Agent.RunStream() → 流式显示响应

---

## 数据流

```
用户输入
  ↓
main.go (readline)
  ↓
Agent.RunStream(ctx, messageCtx, input, onToken)
  ↓
ReAct循环:
  ├─ reader := LLM.Stream(messages) → 流式生成
  ├─ 读取chunks → onToken回调 → 实时显示
  ├─ 检查ToolCalls
  ├─ 有工具调用 → exeTools() → 执行工具 → 添加结果 → 继续循环
  └─ 无工具调用 → 返回完整响应
  ↓
显示完成
```

---

## 关键设计

1. **ReAct模式**: Reasoning (LLM生成) + Acting (工具执行) 循环
2. **流式输出**: Stream API + onToken回调，逐token显示
3. **上下文管理**: 所有消息存储在messageCtx，支持多轮对话
4. **工具调用**: LLM返回ToolCalls → Agent查找工具 → 执行 → 结果回传
5. **日志系统**: 标签分类，彩色输出，格式对齐，内容摘要
6. **最大轮数**: 防止无限循环，默认10轮

---

## 已完成

- ✅ Stage1: ReAct循环，工具调用，多轮对话
- ✅ Stage2: 流式输出，实时显示
- ✅ 日志系统: 标签，颜色，对齐，摘要
- ✅ 工具系统修复: 支持EnhancedInvokableTool接口

## 待实现

- [ ] 工具并发（只读工具并行）
- [ ] 上下文压缩（超长对话）
- [ ] Skill注入
- [ ] 长程任务管理

---

## 问题修复记录

### 工具调用失败问题 (2026-04-18)

**问题**: 工具执行时返回"tool exec_shell is not invokable"错误，导致LLM无法获取工具执行结果。

**原因**: 
- Eino框架有两种工具接口：`InvokableTool`和`EnhancedInvokableTool`
- `InvokableTool.InvokableRun(ctx, argumentsInJSON string) (string, error)` - 接收JSON字符串，返回字符串
- `EnhancedInvokableTool.InvokableRun(ctx, toolArgument *schema.ToolArgument) (*schema.ToolResult, error)` - 接收ToolArgument，返回ToolResult
- 我们的工具使用`utils.InferEnhancedTool`创建，实现的是`EnhancedInvokableTool`接口
- 但agent.go中只尝试断言为`InvokableTool`，导致断言失败

**解决方案** (`agent.go:447-475`):
1. 优先尝试断言为`EnhancedInvokableTool`
2. 如果失败，再尝试`InvokableTool`
3. 添加`formatToolResult()`函数将`*schema.ToolResult`转换为字符串
4. 改进日志输出，显示工具参数和执行结果

**关键代码**:
```go
// 优先尝试 EnhancedInvokableTool
if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
    toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}
    toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
    result = formatToolResult(toolResult)
} else if invokable, ok := t.(tool.InvokableTool); ok {
    result, execErr = invokable.InvokableRun(ctx, tc.Function.Arguments)
}
```
