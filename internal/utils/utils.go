// utils.go - 工具函数
// 功能：加载 prompt 文件，并从 ~/.5hAgent/.env 读取 provider 共存的运行配置。
// 调用方：cmd/5hagent/main.go 经 internal/runtime/runtime.go 调用 LoadConfig。
// 全局状态：读取进程环境变量和用户配置文件；不持有缓存或单例。
package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

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

// LoadOptional reads an optional prompt file <dir>/<name>.md.
// 返回 ok=false 表示文件不存在；其他读取错误会返回给调用方处理。
func LoadOptional(dir, name string) (content string, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("prompt %q not readable: %w", name, err)
	}
	return strings.TrimSpace(string(data)), true, nil
}

// ModelPromptSlug 将 provider/model 编码成稳定的 prompt 文件名片段。
// 规则：小写；字母数字和 '-' 保留；其他字符折叠为单个 '-'。
func ModelPromptSlug(provider, model string) string {
	return promptSlug(provider) + "." + promptSlug(model)
}

// ModelPrefixPromptName 返回当前模型对应的可选 prefix prompt 名称。
func ModelPrefixPromptName(provider, model string) string {
	return "prefix." + ModelPromptSlug(provider, model)
}

// LoadSystemPrompt 加载 main.md，并在存在模型定制 prefix 时将其叠加到 main 前面。
func LoadSystemPrompt(dir, provider, model string) (string, error) {
	mainPrompt, err := Load(dir, "main")
	if err != nil {
		return "", err
	}
	prefixName := ModelPrefixPromptName(provider, model)
	prefix, ok, err := LoadOptional(dir, prefixName)
	if err != nil {
		return "", err
	}
	if !ok || prefix == "" {
		return mainPrompt, nil
	}
	return prefix + "\n\n" + mainPrompt, nil
}

func promptSlug(raw string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		keep := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if keep {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "unknown"
	}
	return slug
}

// =============================================================================
// 配置管理
// =============================================================================

// AppConfig 应用配置根结构。
type AppConfig struct {
	LLM   LLMConfig
	Agent AgentConfig
}

// LLMConfig LLM 提供商配置。
type LLMConfig struct {
	Provider             string
	APIKey               string
	BaseURL              string
	Model                string
	MaxTokens            int
	ThinkingBudgetTokens int
}

// AgentConfig Agent 行为配置。
type AgentConfig struct {
	Name                string
	MaxTotalTokens      int
	RepeatToolLimit     int
	ContextAutoCompress bool
	Debug               bool
}

// 默认值常量。
const (
	DefaultProvider            = "claude"
	DefaultBaseURL             = "https://api.anthropic.com"
	DefaultModel               = "claude-sonnet-4-6"
	DefaultMaxTokens           = 4096
	DefaultAgentName           = "5hAgent"
	DefaultMaxTotalTokens      = 200000
	DefaultRepeatToolLimit     = 5
	DefaultContextAutoCompress = true
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// GetConfigDir 返回 ~/.5hAgent 目录路径。
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".5hAgent"), nil
}

// GetProjectDataDir 返回项目数据目录 ./.5hagent/。
// Task 等与项目相关的数据存储在此。
func GetProjectDataDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}
	return filepath.Join(cwd, ".5hagent"), nil
}

// LoadConfig 从 ~/.5hAgent/.env 加载配置。
// 步骤：读取 env 文件 -> 选择 LLM_PROVIDER -> 加载当前 provider 配置 -> 加载 Agent 配置 -> 校验。
func LoadConfig() (*AppConfig, error) {
	config := defaultConfig()

	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	envPath := filepath.Join(configDir, ".env")
	env, err := readEnvFile(envPath)
	if err != nil {
		return nil, err
	}

	config.LLM = loadProviderLLMConfig(env, config.LLM)
	loadAgentConfig(env, &config.Agent)

	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func readEnvFile(path string) (map[string]string, error) {
	env, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("failed to load .env: %w", err)
	}
	return env, nil
}

func loadProviderLLMConfig(env map[string]string, defaults LLMConfig) LLMConfig {
	provider := strings.ToLower(getEnvValue(env, "LLM_PROVIDER", defaults.Provider))
	cfg := providerDefaults(provider, defaults)
	cfg.Provider = provider
	cfg.APIKey = getProviderEnv(env, provider, "API_KEY", cfg.APIKey)
	cfg.BaseURL = getProviderEnv(env, provider, "BASE_URL", cfg.BaseURL)
	cfg.Model = getProviderEnv(env, provider, "MODEL", cfg.Model)
	if maxTokens := getProviderEnv(env, provider, "MAX_TOKENS", ""); maxTokens != "" {
		if v, err := strconv.Atoi(maxTokens); err == nil {
			cfg.MaxTokens = v
		}
	}
	if budget := getProviderEnv(env, provider, "THINKING_BUDGET_TOKENS", ""); budget != "" {
		if v, err := strconv.Atoi(budget); err == nil {
			cfg.ThinkingBudgetTokens = v
		}
	}
	return cfg
}

func providerDefaults(provider string, defaults LLMConfig) LLMConfig {
	cfg := defaults
	if provider == "openai" {
		cfg.BaseURL = defaultOpenAIBaseURL
		cfg.Model = ""
		cfg.ThinkingBudgetTokens = 0
	}
	return cfg
}

func getProviderEnv(env map[string]string, provider, field, fallback string) string {
	if provider == "" {
		return fallback
	}
	return getEnvValue(env, providerEnvKey(provider, field), fallback)
}

func providerEnvKey(provider, field string) string {
	return "LLM_" + strings.ToUpper(provider) + "_" + field
}

func loadAgentConfig(env map[string]string, config *AgentConfig) {
	config.Name = getEnvValue(env, "AGENT_NAME", config.Name)
	if maxTotal := getEnvValue(env, "AGENT_MAX_TOTAL_TOKENS", ""); maxTotal != "" {
		if v, err := strconv.Atoi(maxTotal); err == nil {
			config.MaxTotalTokens = v
		}
	}
	if repeat := getEnvValue(env, "AGENT_REPEAT_TOOL_LIMIT", ""); repeat != "" {
		if v, err := strconv.Atoi(repeat); err == nil {
			config.RepeatToolLimit = v
		}
	}
	if auto := getEnvValue(env, "AGENT_CONTEXT_AUTO_COMPRESS", ""); auto != "" {
		config.ContextAutoCompress = strings.ToLower(auto) != "false"
	}
}

func getEnvValue(env map[string]string, key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if v := env[key]; v != "" {
		return v
	}
	return fallback
}

func defaultConfig() *AppConfig {
	return &AppConfig{
		LLM: LLMConfig{
			Provider:  DefaultProvider,
			BaseURL:   DefaultBaseURL,
			Model:     DefaultModel,
			MaxTokens: DefaultMaxTokens,
		},
		Agent: AgentConfig{
			Name:                DefaultAgentName,
			MaxTotalTokens:      DefaultMaxTotalTokens,
			RepeatToolLimit:     DefaultRepeatToolLimit,
			ContextAutoCompress: DefaultContextAutoCompress,
			Debug:               false,
		},
	}
}

// Validate 验证当前生效配置。
func (c *AppConfig) Validate() error {
	if err := validateLLMConfig(c.LLM); err != nil {
		return err
	}
	if c.Agent.MaxTotalTokens <= 0 {
		return fmt.Errorf("AGENT_MAX_TOTAL_TOKENS must be positive")
	}
	if c.Agent.RepeatToolLimit <= 0 {
		return fmt.Errorf("AGENT_REPEAT_TOOL_LIMIT must be positive")
	}
	return nil
}

func validateLLMConfig(config LLMConfig) error {
	switch config.Provider {
	case "claude":
		if config.APIKey == "" {
			return fmt.Errorf("LLM_CLAUDE_API_KEY is required for claude provider. Please set in ~/.5hAgent/.env")
		}
	case "openai":
		// OpenAI-compatible 本地服务可使用 dummy key；远端服务按上游要求填写。
	default:
		return fmt.Errorf("unsupported LLM_PROVIDER %q, supported: claude, openai", config.Provider)
	}
	if config.BaseURL == "" {
		return fmt.Errorf("LLM_%s_BASE_URL is required", strings.ToUpper(config.Provider))
	}
	if config.Model == "" {
		return fmt.Errorf("LLM_%s_MODEL is required", strings.ToUpper(config.Provider))
	}
	if config.MaxTokens <= 0 {
		return fmt.Errorf("LLM_%s_MAX_TOKENS must be positive", strings.ToUpper(config.Provider))
	}
	if config.ThinkingBudgetTokens < 0 {
		return fmt.Errorf("LLM_%s_THINKING_BUDGET_TOKENS must be non-negative", strings.ToUpper(config.Provider))
	}
	return nil
}
