# Eino 快速参考 - walle 开发指南

## 1. 核心接口

### 1.1 ChatModel 接口

```go
import "github.com/cloudwego/eino/components/model"

// 基础接口
type BaseChatModel interface {
    Generate(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.StreamReader[*schema.Message], error)
}

// 带工具调用的接口
type ToolCallingChatModel interface {
    BaseChatModel
    WithTools(tools []*schema.ToolInfo) (ToolCallingChatModel, error)
}
```

### 1.2 Tool 接口

```go
import "github.com/cloudwego/eino/components/tool"

// 基础工具
type BaseTool interface {
    Info(ctx context.Context) (*schema.ToolInfo, error)
}

// 标准工具（返回字符串）
type InvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (string, error)
}

// 增强工具（返回多模态结果）
type EnhancedInvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, toolArgument *schema.ToolArgument, opts ...Option) (*schema.ToolResult, error)
}
```

### 1.3 工具创建

```go
import "github.com/cloudwego/eino/components/tool/utils"

// InferEnhancedTool - 自动推断创建增强工具
tool, err := utils.InferEnhancedTool(
    "tool_name",
    "Tool description...",
    func(ctx context.Context, input *MyInput) (*schema.ToolResult, error) {
        // 业务逻辑
        return &schema.ToolResult{
            Parts: []schema.ToolOutputPart{
                {Type: schema.ToolPartTypeText, Text: "result"},
            },
        }, nil
    },
)

// NewEnhancedTool - 手动定义 ToolInfo
toolInfo := &schema.ToolInfo{
    Name: "my_tool",
    Desc: "My custom tool",
    ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
        "arg1": {Type: "string", Desc: "First argument"},
    }),
}
tool := utils.NewEnhancedTool(toolInfo, handlerFunc)
```

### 1.4 ToolsNode - 工具执行器

```go
import "github.com/cloudwego/eino/compose"

// 创建工具节点
toolsNode, err := compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
    Tools: []tool.BaseTool{myTool1, myTool2},

    // 串行执行（默认 false = 并发）
    ExecuteSequentially: false,

    // 未知工具处理
    UnknownToolsHandler: func(ctx context.Context, name, input string) (string, error) {
        return "", fmt.Errorf("unknown tool: %s", name)
    },
})

// 调用工具节点
toolMessages, err := toolsNode.Invoke(ctx, assistantMessageWithToolCalls)
```

### 1.5 Middleware 中间件

```go
import "github.com/cloudwego/eino/compose"

// 工具中间件
toolMiddleware := []compose.ToolMiddleware{{
    EnhancedInvokable: func(next compose.EnhancedInvokableToolEndpoint) compose.EnhancedInvokableToolEndpoint {
        return func(ctx context.Context, input *compose.ToolInput) (*compose.EnhancedInvokableToolOutput, error) {
            log.Printf("Calling: %s", input.Name)
            output, err := next(ctx, input)
            log.Printf("Result: %v", err)
            return output, err
        }
    },
}}

toolsNode, _ := compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
    Tools: tools,
    ToolCallMiddlewares: toolMiddleware,
})
```

### 1.6 Callback 回调

```go
import callbacksHelper "github.com/cloudwego/eino/utils/callbacks"

// 工具回调
toolHandler := &callbacksHelper.ToolCallbackHandler{
    OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
        log.Printf("Tool start: %s", info.Name)
        return ctx
    },
    OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
        log.Printf("Tool end: %s", info.Name)
        return ctx
    },
    OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
        log.Printf("Tool error: %s, %v", info.Name, err)
        return ctx
    },
}

// 模型回调
modelHandler := &callbacksHelper.ModelCallbackHandler{
    OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
        log.Printf("LLM start: %d messages", len(input.Messages))
        return ctx
    },
    OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
        if output.TokenUsage != nil {
            log.Printf("LLM tokens: %d", output.TokenUsage.TotalTokens)
        }
        return ctx
    },
}

// 组合使用
helper := callbacksHelper.NewHandlerHelper().
    ChatModel(modelHandler).
    Tool(toolHandler).
    Handler()

// 应用到运行时
result, err := runnable.Invoke(ctx, input, compose.WithCallbacks(helper))
```

## 2. 常用类型

### 2.1 Message

```go
// schema/message.go
type Message struct {
    Role    RoleType  // system, user, assistant, tool
    Content string
    ToolCalls []ToolCall
    ToolCallID string
    ResponseMeta *ResponseMeta
}

// 创建消息
msg := &schema.Message{
    Role:    schema.User,
    Content: "Hello",
}

// 创建工具调用
tc := schema.ToolCall{
    ID: "call_123",
    Function: schema.FunctionCall{
        Name:      "read_file",
        Arguments: `{"path": "/tmp/test.txt"}`,
    },
}

// 创建工具结果
toolMsg := schema.ToolMessage("file content...", "call_123")
```

### 2.2 ToolCall

```go
// schema/message.go
type ToolCall struct {
    Index *int  // 流式合并用
    ID    string
    Type  string  // 通常是 "function"
    Function FunctionCall
}

type FunctionCall struct {
    Name      string
    Arguments string  // JSON
}
```

### 2.3 ToolResult

```go
// schema/message.go
type ToolResult struct {
    Parts []ToolOutputPart
}

type ToolOutputPart struct {
    Type  ToolPartType  // text, image, audio, video, file
    Text  string
    Image *ToolOutputImage
    Audio *ToolOutputAudio
    Video *ToolOutputVideo
    File  *ToolOutputFile
}

// 返回文本
return &schema.ToolResult{
    Parts: []schema.ToolOutputPart{
        {Type: schema.ToolPartTypeText, Text: "result"},
    },
}, nil
```

### 2.4 ToolInfo

```go
// schema/tool.go
type ToolInfo struct {
    Name string
    Desc string
    ParamsOneOf *ParamsOneOf
}

// 创建参数定义
params := schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
    "path": {Type: "string", Desc: "File path"},
    "limit": {Type: "integer", Desc: "Max lines"},
})
info := &schema.ToolInfo{
    Name: "read_file",
    Desc: "Read file content",
    ParamsOneOf: params,
}
```

## 3. 常见模式

### 3.1 手动创建工具

```go
type ReadFileInput struct {
    Path   string `json:"path"`
    Offset int    `json:"offset,omitempty"`
    Limit  int    `json:"limit,omitempty"`
}

func NewReadFileTool() (tool.BaseTool, error) {
    info := &schema.ToolInfo{
        Name: "read_file",
        Desc: "Read file content",
        ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
            "path":   {Type: "string", Desc: "File path to read"},
            "offset": {Type: "integer", Desc: "Byte offset to start reading"},
            "limit":  {Type: "integer", Desc: "Maximum bytes to read"},
        }),
    }

    return utils.NewEnhancedTool(info, func(ctx context.Context, input *ReadFileInput) (*schema.ToolResult, error) {
        data, err := os.ReadFile(input.Path)
        if err != nil {
            return nil, err
        }

        offset, limit := 0, len(data)
        if input.Offset > 0 {
            offset = input.Offset
        }
        if input.Limit > 0 && input.Limit < len(data)-offset {
            limit = input.Limit
        }

        return &schema.ToolResult{
            Parts: []schema.ToolOutputPart{
                {Type: schema.ToolPartTypeText, Text: string(data[offset:limit])},
            },
        }, nil
    })
}
```

### 3.2 工具并发执行

```go
// ToolsNode 默认并发执行
toolsNode, _ := compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
    Tools: []tool.BaseTool{tool1, tool2, tool3},
    ExecuteSequentially: false,  // 默认并发
})

// 如果需要串行
toolsNode, _ = compose.NewToolsNode(ctx, &compose.ToolsNodeConfig{
    Tools: tools,
    ExecuteSequentially: true,
})
```

### 3.3 带 Token 统计的调用

```go
var totalTokens int

handler := &callbacksHelper.ModelCallbackHandler{
    OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
        if output.TokenUsage != nil {
            totalTokens += output.TokenUsage.TotalTokens
            log.Printf("Total tokens so far: %d", totalTokens)
        }
        return ctx
    },
}

helper := callbacksHelper.NewHandlerHelper().ChatModel(handler).Handler()
result, err := runnable.Invoke(ctx, input, compose.WithCallbacks(helper))
```

## 4. 注意事项

### 4.1 流式读取

```go
stream, err := model.Stream(ctx, messages)
defer stream.Close()

for {
    chunk, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        return nil, err
    }
    // 处理 chunk
    fmt.Print(chunk.Content)
}
```

### 4.2 获取 ToolCall ID

```go
// 在工具函数或中间件中获取当前 ToolCall 的 ID
toolCallID := compose.GetToolCallID(ctx)
```

### 4.3 选项设置

```go
// 模型选项
model.WithTemperature(0.7)
model.WithMaxTokens(2000)
model.WithModel("gpt-4")
model.WithTools(toolInfos)

// 工具选项（自定义）
tool.WithTimeout(30 * time.Second)
```

## 5. 快速对照表

| walle 手写                 | Eino 原生                             |
| -------------------------- | ------------------------------------- |
| `toolCollector`            | `schema.ToolCall.Index` + `ToolsNode` |
| `toolQueue` + `exeToolCall` | `ToolsNode`                          |
| `toolmeta.ReadOnly` 元数据 | `ToolMiddleware`                      |
| `toolRepeatGuard`          | `ToolMiddleware`                      |
| `mergeMeta`                | `model.CallbackOutput.TokenUsage`     |
| `logger.DebugTag`          | `callbacksHelper`                     |

## 6. 参考链接

- [Eino 文档](https://www.cloudwego.io/docs/eino/)
- [ToolsNode 指南](https://www.cloudwego.io/docs/eino/core_modules/components/tools_node_guide/)
- [ChatModel 指南](https://www.cloudwego.io/docs/eino/core_modules/components/chat_model_guide/)
- [Eino Examples](https://github.com/cloudwego/eino-examples)
