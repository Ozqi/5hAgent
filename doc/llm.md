# LLM - 大语言模型客户端

> 由 GPT-5.5 于 2026-06-23 阅读 `internal/llm/client.go`、`internal/utils/utils.go`、`internal/runtime/runtime.go` 后更新。
> 覆盖范围：LLM provider 配置、Claude/OpenAI-compatible 模型初始化、Runtime 与 Agent 的接线。

## 摘要

LLM 模块负责把 `~/.5hAgent/.env` 中当前 provider 的配置转换成 Eino 的 `model.ToolCallingChatModel`。`.env` 可以同时保留 Claude 风格接口和 OpenAI-compatible 接口配置，顶层 `LLM_PROVIDER` 选择当前接口风格；具体模型由该 provider 自己的 `LLM_<PROVIDER>_MODEL` 决定。

## 架构

```mermaid
flowchart LR
    Env[~/.5hAgent/.env] --> Load[utils.LoadConfig]
    Load --> Pick[选择 LLM_PROVIDER]
    Pick --> Active[当前 provider 配置]
    Active --> Runtime[runtime.New]
    Runtime --> Config[llm.Config]
    Config --> Factory[llm.NewClient provider factory]
    Factory --> Claude[eino claude.ChatModel]
    Factory --> OpenAI[eino openai.ChatModel]
    Claude --> Tools[WithTools]
    OpenAI --> Tools
    Tools --> Agent[agent.Agent]
```

## 位置

- [`internal/llm/client.go`](../internal/llm/client.go)：按 provider 创建 Eino 模型。
- [`internal/utils/utils.go`](../internal/utils/utils.go)：读取 provider-specific 配置并校验当前 provider。
- [`internal/runtime/runtime.go`](../internal/runtime/runtime.go)：把当前配置传给 `llm.NewClient`，绑定工具后注入 Agent。

## 核心类型

### Config

```go
type Config struct {
    Provider             string // claude / openai
    APIKey               string // API Key；Ollama OpenAI-compatible 本地模式可填 dummy
    BaseURL              string // Provider endpoint
    Model                string // 模型名称
    MaxTokens            int    // 最大生成 token
    ThinkingBudgetTokens int    // Claude extended thinking 预算；OpenAI 忽略
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

### Provider-specific 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LLM_PROVIDER` | 当前 provider，支持 `claude` / `openai` | `claude` |
| `LLM_CLAUDE_API_KEY` | Claude API Key | - |
| `LLM_CLAUDE_BASE_URL` | Claude Base URL | `https://api.anthropic.com` |
| `LLM_CLAUDE_MODEL` | Claude 模型名 | `claude-sonnet-4-6` |
| `LLM_CLAUDE_MAX_TOKENS` | Claude 最大输出 token | `4096` |
| `LLM_CLAUDE_THINKING_BUDGET_TOKENS` | Claude extended thinking 预算 | `0` |
| `LLM_OPENAI_API_KEY` | OpenAI-compatible API Key；本地 Ollama 可填 `dummy` | - |
| `LLM_OPENAI_BASE_URL` | OpenAI-compatible Base URL | `https://api.openai.com/v1` |
| `LLM_OPENAI_MODEL` | OpenAI-compatible 模型名，可填任意上游支持的模型 | 必填 |
| `LLM_OPENAI_MAX_TOKENS` | OpenAI-compatible 最大生成 token | `4096` |
| `LLM_OPENAI_THINKING_BUDGET_TOKENS` | 保留字段；OpenAI provider 忽略 | `0` |

### Provider 对比

| Provider | API key | Base URL 示例 | Model 示例 | Thinking |
| --- | --- | --- | --- | --- |
| `claude` | 必填 | `https://api.anthropic.com` | `claude-sonnet-4-6` | 支持 |
| `openai` | 远端按需；本地可 dummy | `http://localhost:11434/v1` | `qwen3:14b` | 忽略 |

### 共存配置示例

```env
LLM_PROVIDER=claude

LLM_CLAUDE_API_KEY=your_api_key
LLM_CLAUDE_BASE_URL=https://api.anthropic.com
LLM_CLAUDE_MODEL=claude-sonnet-4-6
LLM_CLAUDE_MAX_TOKENS=4096
LLM_CLAUDE_THINKING_BUDGET_TOKENS=0

LLM_OPENAI_API_KEY=dummy
LLM_OPENAI_BASE_URL=http://localhost:11434/v1
LLM_OPENAI_MODEL=qwen3:14b
LLM_OPENAI_MAX_TOKENS=4096
LLM_OPENAI_THINKING_BUDGET_TOKENS=0
```

切到本地 Ollama 的 OpenAI-compatible 接口：

```env
LLM_PROVIDER=openai
LLM_OPENAI_BASE_URL=http://localhost:11434/v1
LLM_OPENAI_MODEL=qwen3:14b
LLM_OPENAI_API_KEY=dummy
```

换 Ollama 模型只改：

```env
LLM_OPENAI_MODEL=hf.co/bartowski/Qwen_Qwen3.6-27B-GGUF:Q3_K_M
```

## 创建流程

主程序真实入口是 [`runtime.New`](../internal/runtime/runtime.go)：

```go
appConfig, err := utils.LoadConfig()
client, err := llm.NewClient(ctx, &llm.Config{
    Provider:             appConfig.LLM.Provider,
    APIKey:               appConfig.LLM.APIKey,
    BaseURL:              appConfig.LLM.BaseURL,
    Model:                appConfig.LLM.Model,
    MaxTokens:            appConfig.LLM.MaxTokens,
    ThinkingBudgetTokens: appConfig.LLM.ThinkingBudgetTokens,
})

modelWithTools, err := client.GetModel().WithTools(toolInfos)
ag.SetModel(modelWithTools)
```

`llm.NewClientFromEnv` 是包级 legacy/simple helper；主程序配置入口以 `utils.LoadConfig` 为准。

## Thinking 输出

Claude extended thinking 通过 Eino Claude 的 `claude.Thinking` 配置启用。启用后，流式响应中的 thinking delta 会进入 `schema.Message.ReasoningContent`，TUI 将其渲染为独立的浅色 `thinking` 条目。

OpenAI provider 不使用 `LLM_OPENAI_THINKING_BUDGET_TOKENS`。本地模型是否会稳定触发工具调用取决于具体模型能力，建议先用只读任务验证 `WithTools`、工具调用和报告落盘。

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
- [utils.go](../internal/utils/utils.go)
- [runtime.go](../internal/runtime/runtime.go)
