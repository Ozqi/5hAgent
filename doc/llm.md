# LLM - 大语言模型客户端

## 架构

```mermaid
flowchart LR
    subgraph Config["配置"]
        env[".env 或环境变量"]
    end

    subgraph Create["创建"]
        load["godotenv.Load()"]
        config["llm.Config"]
        client["llm.LLMClient"]
    end

    subgraph Model["模型"]
        claude["eino claude.ChatModel"]
        withtools["model.WithTools()"]
        stream["Stream()"]
        generate["Generate()"]
    end

    env --> load
    load --> config
    config --> client
    client --> claude
    claude --> withtools
    withtools --> stream
    withtools --> generate
```

## 位置

- `internal/llm/client.go`
- `internal/llm/client_test.go`

## 核心类型

### Config

```go
type Config struct {
    APIKey               string  // API Key
    BaseURL              string  // Base URL
    Model                string  // 模型名称
    MaxTokens            int     // 最大 token 数
    ThinkingBudgetTokens int     // Claude extended thinking 预算；0 表示关闭
}
```

### LLMClient

```go
type LLMClient struct {
    config *Config
    model  model.ToolCallingChatModel
}
```

## 配置方式

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LLM_API_KEY` | API Key | - |
| `LLM_BASE_URL` | Base URL | `https://api.anthropic.com` |
| `LLM_MODEL` | 模型名 | `claude-sonnet-4-6` |
| `LLM_MAX_TOKENS` | 最大输出 token | `4096` |
| `LLM_THINKING_BUDGET_TOKENS` | Claude extended thinking 预算 token；数值越大，模型可用于思考的 token 越多；`0` 或不设置表示关闭 | `0` |

### .env 文件

```bash
LLM_API_KEY=your_api_key
LLM_BASE_URL=https://api.anthropic.com
LLM_MODEL=claude-sonnet-4-6
LLM_THINKING_BUDGET_TOKENS=2048
```

## 创建流程

```go
// 方式 1：从环境变量
client, err := llm.NewClientFromEnv(ctx, ".env")

// 方式 2：直接创建
client, err := llm.NewClient(ctx, &llm.Config{
    APIKey:               "...",
    BaseURL:              "...",
    Model:                "claude-sonnet-4-6",
    MaxTokens:            4096,
    ThinkingBudgetTokens: 2048,
})
```

## 使用方式

```go
// 获取模型
model := client.GetModel()

// 绑定工具
modelWithTools, err := model.WithTools(toolInfos)

// 流式调用
reader, err := modelWithTools.Stream(ctx, messages)
for {
    chunk, err := reader.Recv()
    if err == io.EOF { break }
    // 处理 chunk
}

// 非流式调用
resp, err := modelWithTools.Generate(ctx, messages)
```

## 与 Agent 的集成

[main.go](cmd/5hagent/main.go)：

```go
// 创建客户端
client, err := llm.NewClient(ctx, &llm.Config{
    APIKey:               appConfig.LLM.APIKey,
    BaseURL:              appConfig.LLM.BaseURL,
    Model:                appConfig.LLM.Model,
    MaxTokens:            appConfig.LLM.MaxTokens,
    ThinkingBudgetTokens: appConfig.LLM.ThinkingBudgetTokens,
})

// 绑定工具
modelWithTools, err := client.GetModel().WithTools(toolInfos)

// 设置到 Agent
ag.SetModel(modelWithTools)
```

## Thinking 输出

Claude extended thinking 通过 Eino Claude 的 `claude.Thinking` 配置启用。启用后，流式响应中的 thinking delta 会进入 `schema.Message.ReasoningContent`，同时 Claude 扩展包也会把 thinking 放在 message extra 中供多轮上下文使用。

Agent 在 `RunStream()` 中读取 `chunk.ReasoningContent`，通过可选 reasoning 回调传给 TUI。TUI 将其渲染为独立的浅色 `thinking` 条目，不混入普通 assistant 正文。

## LLM 压缩

LLM 也用于上下文压缩：

```go
// Agent.RunStream 中
if a.ctxManager.ShouldCompress(messageCtx) {
    a.ctxManager.LMCompress(ctx, messageCtx, a.model, "prompt")
}
```

## 相关代码

- [client.go](../internal/llm/client.go)
- [client_test.go](../internal/llm/client_test.go)
- [main.go](../cmd/5hagent/main.go)
