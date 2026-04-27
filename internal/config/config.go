// config.go - 配置管理
// 功能：统一配置加载、验证、默认值管理
// 主要类型：AppConfig, LLMConfig, AgentConfig, MCPConfig
// 导出函数：Load, GetConfigDir, GetConfigPath
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/lzq/5hAgent/internal/mcp"
	"gopkg.in/yaml.v3"
)

// AppConfig 应用配置根结构
type AppConfig struct {
	LLM   LLMConfig   `yaml:"llm"`
	Agent AgentConfig `yaml:"agent"`
	MCP   MCPConfig   `yaml:"mcp"`
}

// LLMConfig LLM 提供商配置
type LLMConfig struct {
	APIKey    string `yaml:"api_key"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

// AgentConfig Agent 行为配置
type AgentConfig struct {
	Name            string `yaml:"name"`
	MaxTotalTokens  int    `yaml:"max_total_tokens"`
	RepeatToolLimit int    `yaml:"repeat_tool_limit"`
	Debug           bool   `yaml:"debug"`
}

// MCPConfig MCP 服务器配置
type MCPConfig struct {
	Servers []mcp.ServerConfig `yaml:"servers"`
}

// 默认值常量
const (
	DefaultBaseURL         = "https://api.anthropic.com"
	DefaultModel           = "claude-sonnet-4-6"
	DefaultMaxTokens       = 4096
	DefaultAgentName       = "5hAgent"
	DefaultMaxTotalTokens  = 200000
	DefaultRepeatToolLimit = 5
)

// GetConfigDir 返回 ~/.5hAgent 目录路径
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".5hAgent"), nil
}

// GetConfigPath 返回 config.yaml 完整路径
func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load 加载配置，优先级：config.yaml > .env > 默认值
func Load() (*AppConfig, error) {
	config := defaultConfig()

	// 尝试从 .env 加载（向后兼容）
	if err := loadFromEnv(config); err != nil {
		// .env 是可选的，继续
	}

	// 尝试从 config.yaml 加载（覆盖 .env）
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	if err := loadFromYAML(config, configPath); err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在，创建默认配置
			if err := createDefaultConfig(configPath); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// defaultConfig 返回带默认值的配置
func defaultConfig() *AppConfig {
	return &AppConfig{
		LLM: LLMConfig{
			BaseURL:   DefaultBaseURL,
			Model:     DefaultModel,
			MaxTokens: DefaultMaxTokens,
		},
		Agent: AgentConfig{
			Name:            DefaultAgentName,
			MaxTotalTokens:  DefaultMaxTotalTokens,
			RepeatToolLimit: DefaultRepeatToolLimit,
			Debug:           false,
		},
		MCP: MCPConfig{
			Servers: []mcp.ServerConfig{},
		},
	}
}

// loadFromEnv 从 .env 文件加载配置（向后兼容）
func loadFromEnv(config *AppConfig) error {
	// 尝试从当前目录加载 .env
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(".env"); err != nil {
			return err
		}
	}

	// 从环境变量覆盖
	if apiKey := os.Getenv("CLAUDE_API_KEY"); apiKey != "" {
		config.LLM.APIKey = apiKey
	}
	if baseURL := os.Getenv("CLAUDE_BASE_URL"); baseURL != "" {
		config.LLM.BaseURL = baseURL
	}
	if model := os.Getenv("CLAUDE_MODEL"); model != "" {
		config.LLM.Model = model
	}

	return nil
}

// loadFromYAML 从 YAML 文件加载配置
func loadFromYAML(config *AppConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, config)
}

// createDefaultConfig 创建默认 config.yaml 文件
func createDefaultConfig(path string) error {
	config := defaultConfig()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	// 添加头部注释
	header := `# 5hAgent Configuration
# This file is automatically created on first run
# Configuration precedence: config.yaml > .env > defaults

`
	content := header + string(data)

	// 添加 MCP 示例注释
	content += `
# Example MCP server configuration:
# mcp:
#   servers:
#     - name: filesystem
#       command: npx
#       args:
#         - -y
#         - @modelcontextprotocol/server-filesystem
#         - /tmp
#       startup_timeout: 10s
#     - name: brave_search
#       command: npx
#       args:
#         - -y
#         - @modelcontextprotocol/server-brave-search
#       env:
#         BRAVE_API_KEY: your_key_here
#       startup_timeout: 10s
`

	return os.WriteFile(path, []byte(content), 0644)
}

// Validate 验证配置
func (c *AppConfig) Validate() error {
	// 验证 LLM 配置
	if c.LLM.APIKey == "" {
		configPath, _ := GetConfigPath()
		return fmt.Errorf("llm.api_key is required. Please edit %s", configPath)
	}
	if c.LLM.BaseURL == "" {
		return fmt.Errorf("llm.base_url is required")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if c.LLM.MaxTokens <= 0 {
		return fmt.Errorf("llm.max_tokens must be positive")
	}

	// 验证 Agent 配置
	if c.Agent.Name == "" {
		return fmt.Errorf("agent.name is required")
	}
	if c.Agent.MaxTotalTokens <= 0 {
		return fmt.Errorf("agent.max_total_tokens must be positive")
	}
	if c.Agent.RepeatToolLimit <= 0 {
		return fmt.Errorf("agent.repeat_tool_limit must be positive")
	}

	// 验证 MCP 服务器
	for i, server := range c.MCP.Servers {
		if err := server.Validate(); err != nil {
			return fmt.Errorf("mcp.servers[%d]: %w", i, err)
		}
	}

	return nil
}
