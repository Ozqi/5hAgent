# LLM - 大模型客户端封装

## 位置

`internal/llm/client.go` (~107行)

## 概述

LLM 模块提供大模型客户端的封装和配置管理，基于 Eino 框架的 Claude 模型实现。

## 核心组件

### Config 配置结构

```go
type Config struct {
    APIKey    string // API Key
    BaseURL   string // Base URL
    Model     string // 模型名称
    MaxTokens int    // 最大 token 数，默认为 4096
}
```

### LLMClient 客户端

```go
type LLMClient struct {
    config *Config
    model  model.ToolCallingChatModel
}
```

**方法**:
- `GetModel()`: 获取底层的 Eino ToolCallingChatModel
- `GetConfig()`: 获取配置信息

## 核心函数

### NewClientFromEnv(ctx, envPath)

从环境变量创建 LLM 客户端。

**环境变量**:
- `CLAUDE_API_KEY`: API Key（必需）
- `CLAUDE_BASE_URL`: Base URL（可选，默认 https://api.anthropic.com）
- `CLAUDE_MODEL`: 模型名称（可选，默认 claude-sonnet-4-6）

**参数**:
- `ctx`: 上下文
- `envPath`: .env 文件路径（默认 ".env"）

**返回**: LLMClient 实例和可能的错误

**代码链接**: [client.go:43-78](../internal/llm/client.go#L43-L78)

### NewClient(ctx, config)

使用指定配置创建 LLM 客户端。

**参数**:
- `ctx`: 上下文
- `config`: Config 配置实例

**返回**: LLMClient 实例和可能的错误

**功能**:
1. 验证配置（API Key 必需）
2. 设置默认 MaxTokens（4096）
3. 调用 Eino 的 `claude.NewChatModel()` 创建模型
4. 返回封装后的 LLMClient

**代码链接**: [client.go:80-106](../internal/llm/client.go#L80-L106)

## 使用示例

### 从环境变量创建

```go
// cmd/5hagent/main.go
ctx := context.Background()
llmClient, err := llm.NewClientFromEnv(ctx, ".env")
if err != nil {
    log.Fatal(err)
}

model := llmClient.GetModel()
```

### 使用自定义配置

```go
config := &llm.Config{
    APIKey:    "sk-xxx",
    BaseURL:   "https://api.anthropic.com",
    Model:     "claude-sonnet-4-6",
    MaxTokens: 8192,
}

llmClient, err := llm.NewClient(ctx, config)
```

## .env 文件示例

```bash
# .env
CLAUDE_API_KEY=sk-xxx
CLAUDE_BASE_URL=https://api.anthropic.com
CLAUDE_MODEL=claude-sonnet-4-6
```

## 依赖

- **Eino 框架**: `github.com/cloudwego/eino`
- **Eino Claude 扩展**: `github.com/cloudwego/eino-ext/components/model/claude`
- **godotenv**: `github.com/joho/godotenv` - 加载 .env 文件

## 设计特点

### 1. 配置灵活性

- 支持环境变量配置（生产环境）
- 支持代码配置（测试环境）
- 自动加载 .env 文件

### 2. 封装简洁

- 隐藏 Eino 框架细节
- 提供简单的创建接口
- 统一错误处理

### 3. 默认值合理

- BaseURL 默认官方地址
- Model 默认最新 Sonnet 版本
- MaxTokens 默认 4096（平衡性能和成本）

## 与 Agent 的集成

Agent 通过 LLMClient 获取底层模型：

```go
// cmd/5hagent/main.go
llmClient, _ := llm.NewClientFromEnv(ctx, ".env")
model := llmClient.GetModel()

agent := agent.NewAgent(model, tools, config)
```

## 未来扩展

- [ ] 支持多模型切换（Claude, GPT, Gemini）
- [ ] 模型性能监控（延迟、token 使用）
- [ ] 请求重试和错误恢复
- [ ] 流式输出优化
- [ ] 模型缓存和连接池

## 相关文件

- `internal/llm/client.go` - 客户端实现
- `internal/llm/client_test.go` - 单元测试
- `cmd/5hagent/main.go` - 使用示例
- `.env.example` - 环境变量模板
