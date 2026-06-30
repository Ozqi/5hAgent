package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigReadsClaudeProviderConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_PROVIDER=claude",
		"LLM_CLAUDE_API_KEY=provider-key",
		"LLM_CLAUDE_BASE_URL=https://api.anthropic.com",
		"LLM_CLAUDE_MODEL=claude-test",
		"LLM_CLAUDE_MAX_TOKENS=8192",
		"LLM_CLAUDE_THINKING_BUDGET_TOKENS=1024",
	}, "\n"))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.LLM.Provider != "claude" {
		t.Fatalf("Provider = %q, want claude", config.LLM.Provider)
	}
	if config.LLM.APIKey != "provider-key" {
		t.Fatalf("APIKey = %q, want provider-key", config.LLM.APIKey)
	}
	if config.LLM.Model != "claude-test" {
		t.Fatalf("Model = %q, want claude-test", config.LLM.Model)
	}
	if config.LLM.MaxTokens != 8192 {
		t.Fatalf("MaxTokens = %d, want 8192", config.LLM.MaxTokens)
	}
	if config.LLM.ThinkingBudgetTokens != 1024 {
		t.Fatalf("ThinkingBudgetTokens = %d, want 1024", config.LLM.ThinkingBudgetTokens)
	}
}

func TestLoadConfigReadsOpenAIProviderConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_PROVIDER=openai",
		"LLM_OPENAI_API_KEY=provider-key",
		"LLM_OPENAI_BASE_URL=http://localhost:11434/v1",
		"LLM_OPENAI_MODEL=qwen3:14b",
		"LLM_OPENAI_MAX_TOKENS=2048",
	}, "\n"))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.LLM.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", config.LLM.Provider)
	}
	if config.LLM.APIKey != "provider-key" {
		t.Fatalf("APIKey = %q, want provider-key", config.LLM.APIKey)
	}
	if config.LLM.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("BaseURL = %q", config.LLM.BaseURL)
	}
	if config.LLM.Model != "qwen3:14b" {
		t.Fatalf("Model = %q", config.LLM.Model)
	}
	if config.LLM.MaxTokens != 2048 {
		t.Fatalf("MaxTokens = %d, want 2048", config.LLM.MaxTokens)
	}
}

func TestLoadConfigReadsSelectedSupplier(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_SUPPLIER=openrouter",
		"LLM_OPENROUTER_FORMAT=openai",
		"LLM_OPENROUTER_API_KEY=provider-key",
		"LLM_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1",
		"LLM_OPENROUTER_MODEL=openrouter/owl-alpha",
		"LLM_OPENROUTER_MAX_TOKENS=2048",
		"LLM_ANTHROPIC_FORMAT=claude",
		"LLM_ANTHROPIC_API_KEY=anthropic-key",
		"LLM_ANTHROPIC_MODEL=claude-test",
	}, "\n"))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.LLM.Supplier != "openrouter" {
		t.Fatalf("Supplier = %q, want openrouter", config.LLM.Supplier)
	}
	if config.LLM.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai format", config.LLM.Provider)
	}
	if config.LLM.APIKey != "provider-key" {
		t.Fatalf("APIKey = %q, want provider-key", config.LLM.APIKey)
	}
	if config.LLM.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("BaseURL = %q", config.LLM.BaseURL)
	}
	if config.LLM.Model != "openrouter/owl-alpha" {
		t.Fatalf("Model = %q", config.LLM.Model)
	}
	if config.LLM.MaxTokens != 2048 {
		t.Fatalf("MaxTokens = %d, want 2048", config.LLM.MaxTokens)
	}
}

func TestLoadConfigOptionsOverrideSupplier(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_SUPPLIER=openrouter",
		"LLM_OPENROUTER_FORMAT=openai",
		"LLM_OPENROUTER_API_KEY=router-key",
		"LLM_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1",
		"LLM_OPENROUTER_MODEL=openrouter/owl-alpha",
		"LLM_ANTHROPIC_FORMAT=claude",
		"LLM_ANTHROPIC_API_KEY=anthropic-key",
		"LLM_ANTHROPIC_BASE_URL=https://api.anthropic.com",
		"LLM_ANTHROPIC_MODEL=claude-test",
		"LLM_ANTHROPIC_THINKING_BUDGET_TOKENS=1024",
	}, "\n"))

	config, err := LoadConfigWithOptions(LoadConfigOptions{LLMSupplier: "anthropic"})
	if err != nil {
		t.Fatalf("LoadConfigWithOptions() error = %v", err)
	}
	if config.LLM.Supplier != "anthropic" {
		t.Fatalf("Supplier = %q, want anthropic", config.LLM.Supplier)
	}
	if config.LLM.Provider != "claude" {
		t.Fatalf("Provider = %q, want claude format", config.LLM.Provider)
	}
	if config.LLM.APIKey != "anthropic-key" {
		t.Fatalf("APIKey = %q, want anthropic-key", config.LLM.APIKey)
	}
	if config.LLM.ThinkingBudgetTokens != 1024 {
		t.Fatalf("ThinkingBudgetTokens = %d, want 1024", config.LLM.ThinkingBudgetTokens)
	}
}

func TestLoadConfigOptionsOverrideFormatAndModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_SUPPLIER=openrouter",
		"LLM_OPENROUTER_FORMAT=openai",
		"LLM_OPENROUTER_API_KEY=router-key",
		"LLM_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1",
		"LLM_OPENROUTER_MODEL=openrouter/owl-alpha",
	}, "\n"))

	config, err := LoadConfigWithOptions(LoadConfigOptions{LLMFormat: "openai", LLMModel: "openrouter/auto"})
	if err != nil {
		t.Fatalf("LoadConfigWithOptions() error = %v", err)
	}
	if config.LLM.Supplier != "openrouter" {
		t.Fatalf("Supplier = %q, want openrouter", config.LLM.Supplier)
	}
	if config.LLM.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai format", config.LLM.Provider)
	}
	if config.LLM.Model != "openrouter/auto" {
		t.Fatalf("Model = %q", config.LLM.Model)
	}
	if config.LLM.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("BaseURL = %q", config.LLM.BaseURL)
	}
}

func TestLoadConfigRejectsSupplierWithoutFormat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, "LLM_SUPPLIER=broken\nLLM_BROKEN_MODEL=qwen3:14b\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want supplier format error")
	}
	if !strings.Contains(err.Error(), "LLM_BROKEN_FORMAT") {
		t.Fatalf("LoadConfig() error = %v, want supplier format key", err)
	}
}

func TestLoadConfigRejectsMissingProviderSpecificKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_PROVIDER=claude",
		"LLM_API_KEY=legacy-key",
		"LLM_MODEL=legacy-model",
	}, "\n"))

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want provider-specific API key error")
	}
	if !strings.Contains(err.Error(), "LLM_CLAUDE_API_KEY") {
		t.Fatalf("LoadConfig() error = %v, want LLM_CLAUDE_API_KEY", err)
	}
}

func TestLoadConfigIgnoresInactiveProviderValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_PROVIDER=claude",
		"LLM_CLAUDE_API_KEY=test-key",
		"LLM_CLAUDE_MODEL=claude-test",
		"LLM_OPENAI_MODEL=",
	}, "\n"))

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestLoadConfigRejectsOpenAIWithoutModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, "LLM_PROVIDER=openai\nLLM_OPENAI_MODEL=\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want model error")
	}
	if !strings.Contains(err.Error(), "LLM_OPENAI_MODEL") {
		t.Fatalf("LoadConfig() error = %v, want LLM_OPENAI_MODEL", err)
	}
}

func TestLoadConfigRejectsClaudeWithoutAPIKey(t *testing.T) {
	config := defaultConfig()
	config.LLM.Provider = "claude"
	config.LLM.APIKey = ""

	err := config.Validate()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want API key error")
	}
	if !strings.Contains(err.Error(), "LLM_CLAUDE_API_KEY") {
		t.Fatalf("LoadConfig() error = %v, want LLM_CLAUDE_API_KEY", err)
	}
}

func TestLoadConfigRejectsUnsupportedProvider(t *testing.T) {
	config := defaultConfig()
	config.LLM.Provider = "bad"
	config.LLM.APIKey = "test-key"

	err := config.Validate()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want provider error")
	}
	if !strings.Contains(err.Error(), "unsupported LLM format") {
		t.Fatalf("LoadConfig() error = %v, want unsupported provider", err)
	}
}

func TestLoadConfigNormalizesProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_PROVIDER=OPENAI",
		"LLM_OPENAI_BASE_URL=http://localhost:11434/v1",
		"LLM_OPENAI_MODEL=qwen2.5-coder:7b",
	}, "\n"))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.LLM.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", config.LLM.Provider)
	}
}

func TestLoadConfigRejectsOllamaProvider(t *testing.T) {
	config := defaultConfig()
	config.LLM.Provider = "ollama"
	config.LLM.Model = "qwen3:14b"

	err := config.Validate()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want provider error")
	}
	if !strings.Contains(err.Error(), "supported: claude, openai") {
		t.Fatalf("LoadConfig() error = %v, want claude/openai provider list", err)
	}
}

func writeConfig(t *testing.T, home, env string) {
	t.Helper()
	configDir := filepath.Join(home, ".5hAgent")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if !strings.HasSuffix(env, "\n") {
		env += "\n"
	}
	if err := os.WriteFile(filepath.Join(configDir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
