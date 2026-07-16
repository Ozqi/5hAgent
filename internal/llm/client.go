// client.go - LLM 客户端封装
// 功能：按 provider 创建 Eino ToolCallingChatModel，并屏蔽 Claude/OpenAI 风格接口的初始化差异。
// 调用方：internal/runtime/runtime.go 负责加载配置、绑定工具并注入 Agent。
// 全局状态：无；LLMClient 只持有一次运行时使用的模型实例和配置快照。
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

const (
	ProviderClaude = "claude"
	ProviderOpenAI = "openai"
)

// Config 描述一次 LLM provider 初始化所需配置。
// Provider 选择底层模型适配器；ThinkingBudgetTokens 仅对 Claude 生效。
type Config struct {
	Provider             string // claude / openai
	APIKey               string // API Key；Ollama OpenAI-compatible 本地模式可填 dummy
	BaseURL              string // provider endpoint，例如 Anthropic 或 http://localhost:11434/v1
	Model                string // 模型名称
	MaxTokens            int    // 最大生成 token 数
	ThinkingBudgetTokens int    // Claude extended thinking 预算；OpenAI 忽略；0 表示关闭
}

// Client LLM 客户端封装
type LLMClient struct {
	config *Config
	model  model.ToolCallingChatModel
}

// GetModel 获取底层的 Eino ToolCallingChatModel
func (c *LLMClient) GetModel() model.ToolCallingChatModel {
	return c.model
}

// NewClient 根据 Config.Provider 创建 LLM 客户端。
// 步骤：规范化 provider -> 选择 provider -> 构造 Eino ToolCallingChatModel。
func NewClient(ctx context.Context, config *Config) (*LLMClient, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	config.Provider = strings.ToLower(strings.TrimSpace(config.Provider))
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}

	chatModel, err := buildModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return &LLMClient{config: config, model: chatModel}, nil
}

func buildModel(ctx context.Context, config *Config) (model.ToolCallingChatModel, error) {
	switch config.Provider {
	case ProviderClaude:
		return newClaudeModel(ctx, config)
	case ProviderOpenAI:
		return newOpenAIModel(ctx, config)
	default:
		return nil, fmt.Errorf("unsupported LLM provider %q, supported: claude, openai", config.Provider)
	}
}

func newClaudeModel(ctx context.Context, config *Config) (model.ToolCallingChatModel, error) {
	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		APIKey:    config.APIKey,
		BaseURL:   &config.BaseURL,
		Model:     config.Model,
		MaxTokens: config.MaxTokens,
		Thinking:  thinkingConfig(config.ThinkingBudgetTokens),
	})
	if err != nil {
		return nil, fmt.Errorf("create claude model: %w", err)
	}
	return chatModel, nil
}

func newOpenAIModel(ctx context.Context, config *Config) (model.ToolCallingChatModel, error) {
	maxTokens := config.MaxTokens
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:    config.APIKey,
		BaseURL:   config.BaseURL,
		Model:     config.Model,
		MaxTokens: &maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai model: %w", err)
	}
	return chatModel, nil
}

func thinkingConfig(budgetTokens int) *claude.Thinking {
	if budgetTokens <= 0 {
		return nil
	}
	return &claude.Thinking{Enable: true, BudgetTokens: budgetTokens}
}
