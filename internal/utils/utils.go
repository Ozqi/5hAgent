// utils.go - 工具函数
// 功能：
//  - 加载 prompt 目录下的 .md 文件内容
//  - 从 ~/.5hAgent/.env 加载配置
// 导出函数：Load, LoadConfig, GetConfigDir, GetProjectDataDir
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// =============================================================================
// Prompt 加载
// =============================================================================

// Load reads prompt file <dir>/<name>.md and returns its trimmed content.
func Load(dir, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		return "", fmt.Errorf("prompt %q not found: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// =============================================================================
// 配置管理
// =============================================================================

// AppConfig 应用配置根结构
type AppConfig struct {
	LLM   LLMConfig
	Agent AgentConfig
}

// LLMConfig LLM 提供商配置
type LLMConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	MaxTokens int
}

// AgentConfig Agent 行为配置
type AgentConfig struct {
	Name            string
	MaxTotalTokens  int
	RepeatToolLimit int
	Debug           bool
}

// 默认值常量
const (
	DefaultBaseURL         = "https://api.anthropic.com"
	DefaultModel          = "claude-sonnet-4-6"
	DefaultMaxTokens      = 4096
	DefaultAgentName      = "5hAgent"
	DefaultMaxTotalTokens = 200000
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

// GetProjectDataDir 返回项目数据目录 ./5hagent/
// Task 等与项目相关的数据存储在此
func GetProjectDataDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}
	return filepath.Join(cwd, ".5hagent"), nil
}

// LoadConfig 从 ~/.5hAgent/.env 加载配置
func LoadConfig() (*AppConfig, error) {
	config := defaultConfig()

	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	envPath := filepath.Join(configDir, ".env")
	if err := godotenv.Load(envPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load .env: %w", err)
	}

	config.LLM.APIKey = getEnv("LLM_API_KEY", config.LLM.APIKey)
	config.LLM.BaseURL = getEnv("LLM_BASE_URL", config.LLM.BaseURL)
	config.LLM.Model = getEnv("LLM_MODEL", config.LLM.Model)
	if maxTokens := getEnv("LLM_MAX_TOKENS", ""); maxTokens != "" {
		if v, err := strconv.Atoi(maxTokens); err == nil {
			config.LLM.MaxTokens = v
		}
	}

	config.Agent.Name = getEnv("AGENT_NAME", config.Agent.Name)
	if maxTotal := getEnv("AGENT_MAX_TOTAL_TOKENS", ""); maxTotal != "" {
		if v, err := strconv.Atoi(maxTotal); err == nil {
			config.Agent.MaxTotalTokens = v
		}
	}
	if repeat := getEnv("AGENT_REPEAT_TOOL_LIMIT", ""); repeat != "" {
		if v, err := strconv.Atoi(repeat); err == nil {
			config.Agent.RepeatToolLimit = v
		}
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

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
	}
}

// Validate 验证配置
func (c *AppConfig) Validate() error {
	if c.LLM.APIKey == "" {
		return fmt.Errorf("LLM_API_KEY is required. Please set in ~/.5hAgent/.env")
	}
	if c.LLM.BaseURL == "" {
		return fmt.Errorf("LLM_BASE_URL is required")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("LLM_MODEL is required")
	}
	if c.LLM.MaxTokens <= 0 {
		return fmt.Errorf("LLM_MAX_TOKENS must be positive")
	}
	if c.Agent.MaxTotalTokens <= 0 {
		return fmt.Errorf("AGENT_MAX_TOTAL_TOKENS must be positive")
	}
	if c.Agent.RepeatToolLimit <= 0 {
		return fmt.Errorf("AGENT_REPEAT_TOOL_LIMIT must be positive")
	}
	return nil
}
