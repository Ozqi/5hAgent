# miniAgent 架构文档

> 代码量：~1000行 | 技术栈：Go 1.23 + Eino + Claude API

**相关文档**:
- [Eino框架使用说明](./eino_usage.md)
- [Agent模块详解](./agent.md)
- [Stage1代码Review](./stage1_review.md)

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

**agent.go** (260行)

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
- `Run(ctx, messageCtx, input)`: ReAct循环主入口
  1. 注入SystemPrompt(首次对话)
  2. 添加用户消息到messageCtx
  3. 循环(最多MaxTurns轮):
     - 调用LLM生成响应
     - 检查是否有工具调用
     - 有工具调用 → 执行工具 → 继续循环
     - 无工具调用 → 返回响应
- `exeTools(ctx, messageCtx, toolCalls)`: 执行工具调用列表
- `findTool(name)`: 从toolMap查找工具(O(1))

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

### 5. CLI (`internal/cli/` + `cmd/miniagent/`)

**ui.go**: CLI输出函数
- `PrintError(err)`: 打印错误
- `PrintAssistantChunk(text)`: 打印助手响应

**main.go** (160行): 主入口
1. 加载.env配置
2. 创建LLM客户端
3. 注册工具
4. 创建Agent
5. 创建上下文管理器
6. 启动readline交互循环
7. 处理用户输入 → Agent.Run() → 显示响应

---

## 数据流

```
用户输入
  ↓
main.go (readline)
  ↓
Agent.Run(ctx, messageCtx, input)
  ↓
ReAct循环:
  ├─ LLM.Generate(messages) → 生成响应
  ├─ 检查ToolCalls
  ├─ 有工具调用 → exeTools() → 执行工具 → 添加结果 → 继续循环
  └─ 无工具调用 → 返回响应
  ↓
显示响应
```

---

## 关键设计

1. **ReAct模式**: Reasoning (LLM生成) + Acting (工具执行) 循环
2. **上下文管理**: 所有消息存储在messageCtx，支持多轮对话
3. **工具调用**: LLM返回ToolCalls → Agent查找工具 → 执行 → 结果回传
4. **最大轮数**: 防止无限循环，默认10轮

---

## Stage1 完成优化

- ✅ SystemPrompt自动注入
- ✅ Manager实例复用
- ✅ 工具查找优化(map O(1))

## Phase2 计划

- Streaming输出（逐token显示）
- 工具并发（只读工具并行）
- 上下文压缩（超长对话）
- 更多工具（glob, edit）
