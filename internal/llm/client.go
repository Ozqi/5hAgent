// Package llm 提供 LLM 模型的封装和配置管理
package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
	"github.com/joho/godotenv"
)

// Config LLM 配置
type Config struct {
	APIKey    string // API Key
	BaseURL   string // Base URL
	Model     string // 模型名称
	MaxTokens int    // 最大 token 数，默认为 4096
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

// GetConfig 获取配置
func (c *LLMClient) GetConfig() *Config {
	return c.config
}

// NewClientFromEnv 从环境变量创建 LLM 客户端
// 支持的环境变量：
// - CLAUDE_API_KEY: API Key（必需）
// - CLAUDE_BASE_URL: Base URL（可选，默认 https://api.anthropic.com）
// - CLAUDE_MODEL: 模型名称（可选，默认 claude-sonnet-4-6）
func NewClientFromEnv(ctx context.Context, envPath string) (*LLMClient, error) {
	// 加载 .env 文件
	if envPath == "" {
		envPath = ".env"
	}
	if _, err := os.Stat(envPath); err == nil {
		if err := godotenv.Load(envPath); err != nil {
			return nil, fmt.Errorf("failed to load .env file: %w", err)
		}
	}

	// 从环境变量读取配置
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("CLAUDE_API_KEY environment variable is required")
	}

	baseURL := os.Getenv("CLAUDE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	modelName := os.Getenv("CLAUDE_MODEL")
	if modelName == "" {
		modelName = "claude-sonnet-4-6"
	}

	config := &Config{
		APIKey:    apiKey,
		BaseURL:   baseURL,
		Model:     modelName,
		MaxTokens: 4096,
	}

	return NewClient(ctx, config)
}

// NewClient 创建 LLM 客户端
func NewClient(ctx context.Context, config *Config) (*LLMClient, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}

	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		APIKey:    config.APIKey,
		BaseURL:   &config.BaseURL,
		Model:     config.Model,
		MaxTokens: config.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	return &LLMClient{
		config: config,
		model:  chatModel,
	}, nil
}
