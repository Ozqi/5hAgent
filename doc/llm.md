# LLM - 客户端封装

```text
.env / env vars
  -> internal/llm/client.go NewClientFromEnv()
  -> NewClient()
  -> claude.NewChatModel(...)
```

## 位置

- `internal/llm/client.go`
- `internal/llm/client_test.go`
- `.env.example`

## 概述

当前 LLM 封装很薄，职责只有两层：

- 从环境变量构造 `Config`
- 调用 Eino Claude 兼容 chat model

## 核心类型

### `Config`

- `APIKey`
- `BaseURL`
- `Model`
- `MaxTokens`

### `LLMClient`

- `GetModel()` 返回底层 `model.ToolCallingChatModel`
- `GetConfig()` 返回配置

## 环境变量

`NewClientFromEnv()` 读取：

- `CLAUDE_API_KEY`
- `CLAUDE_BASE_URL`
- `CLAUDE_MODEL`

默认值：

- `CLAUDE_BASE_URL=https://api.anthropic.com`
- `CLAUDE_MODEL=claude-sonnet-4-6`
- `MaxTokens=4096`

注意：虽然 README 中提到“Claude API 兼容格式”，当前代码默认值仍指向 Anthropic 官方地址；如果要接别的兼容服务，需要自行修改 `.env`。

## 创建流程

1. 可选加载 `.env`
2. 读取环境变量
3. 组装 `Config`
4. 调用 `claude.NewChatModel(...)`
5. 返回 `LLMClient`

## 使用位置

主程序中由 `cmd/5hagent/main.go` 调用：

```go
client, err := llm.NewClientFromEnv(ctx, ".env")
modelWithTools, err := client.GetModel().WithTools(toolInfos)
```

## 相关代码

- [client.go](../internal/llm/client.go)
- [client_test.go](../internal/llm/client_test.go)
- [main.go](../cmd/5hagent/main.go)
- [env example](../.env.example)
