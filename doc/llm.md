# LLM - 大语言模型客户端

> 由 GPT-5.5 于 2026-06-23 阅读 `internal/llm/client.go`、`internal/utils/utils.go`、`internal/runtime/runtime.go` 后更新。
> 覆盖范围：LLM provider 配置、Claude/OpenAI-compatible 模型初始化、Runtime 与 Agent 的接线。

## 摘要

LLM 模块负责把 `~/.5hAgent/.env` 中当前模型配置转换成 Eino 的 `model.ToolCallingChatModel`。当前配置只认一条主线：`LLM_MODEL=provider/model` 选择 provider 和模型名，`LLM_<PROVIDER>_*` 保存该 provider 的 API 地址、密钥和接口格式。

## 架构

```mermaid
flowchart LR
    Env[~/.5hAgent/.env] --> Load[utils.LoadConfigWithOptions]
    CLI[CLI override] --> Load
    Load --> Pick[解析 LLM_MODEL provider/model]
    Pick --> Active[当前 LLM 配置]
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

- [`internal/llm/client.go`](../internal/llm/client.go)：按接口格式创建 Eino 模型。
- [`internal/utils/utils.go`](../internal/utils/utils.go)：读取 `LLM_MODEL` 和当前 provider 配置并校验当前生效配置。
- [`internal/runtime/runtime.go`](../internal/runtime/runtime.go)：把当前配置传给 `llm.NewClient`，绑定工具后注入 Agent。

## 核心类型

### Config

```go
type Config struct {
    Provider             string // 接口格式：claude / openai
    APIKey               string // API Key；Ollama OpenAI-compatible 本地模式可填 dummy
    BaseURL              string // 供应商 endpoint
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

### Provider 环境变量

Provider 名会转成大写下划线形式：`openrouter` 对应 `LLM_OPENROUTER_*`，`moonshot-ai` 对应 `LLM_MOONSHOT_AI_*`。

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LLM_MODEL` | 当前模型，格式为 `provider/model`；第一段选择 provider 配置块，剩余部分作为上游模型名 | 必填 |
| `LLM_<PROVIDER>_FORMAT` | 当前 provider 的接口格式，支持 `claude` / `openai` | 必填 |
| `LLM_<PROVIDER>_API_KEY` | 当前 provider 的 API Key | Claude format 必填；OpenAI-compatible 按上游要求 |
| `LLM_<PROVIDER>_BASE_URL` | 当前 provider 的 Base URL | format 默认值 |
| `LLM_<PROVIDER>_MAX_TOKENS` | 当前 provider 最大生成 token | `4096` |
| `LLM_<PROVIDER>_THINKING_BUDGET_TOKENS` | Claude extended thinking 预算 | `0` |
| `LLM_<PROVIDER>_STREAM` | 是否使用流式调用；`false` 时走非流式 `Generate` | `true` |

### 接口格式对比

| Format | API key | Base URL 示例 | Model 示例 | Thinking |
| --- | --- | --- | --- | --- |
| `claude` | 必填 | `https://api.anthropic.com` | `claude-sonnet-4-6` | 支持 |
| `openai` | 远端按需；本地可 dummy | `http://localhost:11434/v1` | `qwen3:14b` | 忽略 |

### 多 Provider 配置示例

```env
LLM_MODEL=openrouter/openrouter/owl-alpha

LLM_OPENROUTER_FORMAT=openai
LLM_OPENROUTER_API_KEY=your_api_key
LLM_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1
LLM_OPENROUTER_MAX_TOKENS=4096
LLM_OPENROUTER_STREAM=false

LLM_ANTHROPIC_FORMAT=claude
LLM_ANTHROPIC_API_KEY=your_api_key
LLM_ANTHROPIC_BASE_URL=https://api.anthropic.com
LLM_ANTHROPIC_MAX_TOKENS=4096
LLM_ANTHROPIC_THINKING_BUDGET_TOKENS=0
```

临时切到另一个 provider/model：

```bash
5hagent --model openrouter/openrouter/owl-alpha run
```

也可以只临时覆盖当前 provider 的接口格式或模型名：

```bash
5hagent --llm-format openai --llm-model openrouter/owl-alpha run
```

Ollama 作为普通 provider：

```env
LLM_MODEL=ollama/qwen3:14b
LLM_OLLAMA_FORMAT=openai
LLM_OLLAMA_BASE_URL=http://localhost:11434/v1
LLM_OLLAMA_API_KEY=dummy
```

## 创建流程

主程序真实入口是 [`runtime.New`](../internal/runtime/runtime.go)：

```go
appConfig, err := utils.LoadConfigWithOptions(utils.LoadConfigOptions{
    LLMFormat:   opts.LLMFormat,
    LLMModel:    opts.LLMModel,
    ModelRef:    opts.ModelRef,
})
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

OpenAI format 不使用 thinking budget。本地模型是否会稳定触发工具调用取决于具体模型能力，建议先用只读任务验证 `WithTools`、工具调用和报告落盘。

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
