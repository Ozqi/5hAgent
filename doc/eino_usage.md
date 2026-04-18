# Eino 框架使用说明

> Eino: CloudWeGo 开源的 LLM Application 框架

## 项目中使用的 Eino 模块

### 1. components/model

**包路径**: `github.com/cloudwego/eino/components/model`

**使用位置**: `internal/llm/client.go`, `internal/agent/agent.go`

**核心接口**:
```go
type ToolCallingChatModel interface {
    Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
}
```

**功能**: LLM 模型接口，支持工具调用
- `Generate()`: 输入消息列表，返回 LLM 响应（可能包含 ToolCalls）

**使用方式**:
```go
// 创建模型
model := claude.NewChatModel(ctx, config)

// 调用生成
resp, err := model.Generate(ctx, messages)
```

---

### 2. components/tool

**包路径**: `github.com/cloudwego/eino/components/tool`

**使用位置**: `internal/tools/*.go`, `internal/agent/agent.go`

**核心接口**:
```go
type BaseTool interface {
    Info(ctx context.Context) (*ToolInfo, error)
}

type InvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, argumentsInJSON string) (string, error)
}
```

**功能**: 工具定义和执行
- `Info()`: 返回工具元信息（名称、描述、参数schema）
- `InvokableRun()`: 执行工具，输入JSON参数，返回字符串结果

**使用方式**:
```go
// 定义工具
type ReadFileTool struct {
    tool.BaseTool
}

func (t *ReadFileTool) InvokableRun(ctx context.Context, args string) (string, error) {
    // 解析参数
    var params struct { Path string `json:"path"` }
    json.Unmarshal([]byte(args), &params)
    
    // 执行逻辑
    content, err := os.ReadFile(params.Path)
    return string(content), err
}
```

---

### 3. components/tool/utils

**包路径**: `github.com/cloudwego/eino/components/tool/utils`

**使用位置**: `internal/tools/read_file.go`, `internal/tools/exec_shell.go`

**功能**: 工具构建辅助函数

**核心函数**:
```go
func NewTool(info *schema.ToolInfo, fn func(ctx context.Context, args string) (string, error)) tool.InvokableTool
```

**使用方式**:
```go
// 快速创建工具
readTool := utils.NewTool(
    &schema.ToolInfo{
        Name: "read_file",
        Desc: "读取文件内容",
        ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
            "path": {Type: "string", Desc: "文件路径", Required: true},
        }),
    },
    func(ctx context.Context, args string) (string, error) {
        // 实现逻辑
    },
)
```

---

### 4. schema

**包路径**: `github.com/cloudwego/eino/schema`

**使用位置**: 所有模块

**核心类型**:
```go
// 消息
type Message struct {
    Role      string      // User/Assistant/System/Tool
    Content   string      // 消息内容
    ToolCalls []ToolCall  // 工具调用列表（Assistant消息）
}

// 工具调用
type ToolCall struct {
    ID       string
    Type     string
    Function struct {
        Name      string
        Arguments string // JSON格式
    }
}

// 工具信息
type ToolInfo struct {
    Name         string
    Desc         string
    ParamsOneOf  *ParamsOneOf  // 参数schema
}

// 参数信息
type ParameterInfo struct {
    Type     string
    Desc     string
    Required bool
}
```

**角色常量**:
- `schema.User`: 用户消息
- `schema.Assistant`: 助手消息
- `schema.System`: 系统提示词
- `schema.Tool`: 工具执行结果

**辅助函数**:
```go
// 创建工具结果消息
msg := schema.ToolMessage(result, toolCallID)

// 创建参数schema
params := schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
    "path": {Type: "string", Desc: "文件路径", Required: true},
})
```

---

### 5. eino-ext/components/model/claude

**包路径**: `github.com/cloudwego/eino-ext/components/model/claude`

**使用位置**: `internal/llm/client.go`

**功能**: Claude API 客户端实现

**配置**:
```go
type ChatModelConfig struct {
    Model   string  // 模型名称
    APIKey  string  // API密钥
    BaseURL string  // API地址
}
```

**创建方式**:
```go
model, err := claude.NewChatModel(ctx, &claude.ChatModelConfig{
    Model:   "claude-3-5-sonnet-20241022",
    APIKey:  os.Getenv("CLAUDE_API_KEY"),
    BaseURL: os.Getenv("CLAUDE_BASE_URL"),
})
```

---

## 数据流

```
用户输入
  ↓
[schema.Message] Role=User
  ↓
Agent.Run() → model.Generate(messages)
  ↓
[schema.Message] Role=Assistant + ToolCalls
  ↓
Agent.exeTools() → tool.InvokableRun(args)
  ↓
[schema.Message] Role=Tool (schema.ToolMessage)
  ↓
继续循环 → model.Generate(messages)
  ↓
[schema.Message] Role=Assistant (无ToolCalls)
  ↓
返回响应
```

---

## 关键设计

### 1. 消息历史管理
所有消息（User/Assistant/Tool）按顺序存储在 `[]*schema.Message` 中，每次调用 `model.Generate()` 时传入完整历史。

### 2. 工具调用流程
1. LLM 返回 `ToolCalls` 列表
2. Agent 遍历 `ToolCalls`，查找对应工具
3. 调用 `tool.InvokableRun()`，传入 JSON 参数
4. 将结果封装为 `schema.ToolMessage`
5. 添加到消息历史，继续循环

### 3. 工具注册
工具实现 `tool.InvokableTool` 接口，通过 `Info()` 返回 schema，LLM 根据 schema 决定何时调用。

### 4. 工具定义两种方式

**方式1: 使用 utils.NewTool（推荐）**
```go
tool := utils.NewTool(toolInfo, handlerFunc)
```

**方式2: 实现 InvokableTool 接口**
```go
type MyTool struct {
    tool.BaseTool
}

func (t *MyTool) InvokableRun(ctx context.Context, args string) (string, error) {
    // 实现
}
```

---

## 参考

- Eino GitHub: https://github.com/cloudwego/eino
- Eino-ext GitHub: https://github.com/cloudwego/eino-ext
- Claude API: https://docs.anthropic.com/claude/reference
