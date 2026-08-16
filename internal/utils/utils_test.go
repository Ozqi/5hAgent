package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSystemPromptBaseUsesRequestedBase(t *testing.T) {
	dir := t.TempDir()
	writePrompt(t, dir, "main", "main prompt")
	writePrompt(t, dir, "tui", "tui prompt")
	writePrompt(t, dir, "prefix.mira.gpt-5-4", "model prefix")

	prompt, err := LoadSystemPromptBase(dir, "tui", "mira", "gpt-5.4")
	if err != nil {
		t.Fatalf("LoadSystemPromptBase() error = %v", err)
	}
	if prompt != "model prefix\n\ntui prompt" {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestLoadSystemPromptBaseFallsBackToMain(t *testing.T) {
	dir := t.TempDir()
	writePrompt(t, dir, "main", "main prompt")

	prompt, err := LoadSystemPromptBase(dir, "tui", "mira", "gpt-5.4")
	if err != nil {
		t.Fatalf("LoadSystemPromptBase() error = %v", err)
	}
	if prompt != "main prompt" {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestLoadConfigReadsProviderModelRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_MODEL=mira/claude-opus-4-6",
		"LLM_MIRA_FORMAT=claude",
		"LLM_MIRA_API_KEY=provider-key",
		"LLM_MIRA_BASE_URL=http://127.0.0.1:8787",
		"LLM_MIRA_MAX_TOKENS=8192",
		"LLM_MIRA_THINKING_BUDGET_TOKENS=1024",
		"LLM_MIRA_STREAM=false",
	}, "\n"))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.LLM.Supplier != "mira" {
		t.Fatalf("Supplier = %q, want mira", config.LLM.Supplier)
	}
	if config.LLM.Provider != "claude" {
		t.Fatalf("Provider = %q, want claude format", config.LLM.Provider)
	}
	if config.LLM.APIKey != "provider-key" {
		t.Fatalf("APIKey = %q, want provider-key", config.LLM.APIKey)
	}
	if config.LLM.BaseURL != "http://127.0.0.1:8787" {
		t.Fatalf("BaseURL = %q", config.LLM.BaseURL)
	}
	if config.LLM.Model != "claude-opus-4-6" {
		t.Fatalf("Model = %q, want claude-opus-4-6", config.LLM.Model)
	}
	if config.LLM.MaxTokens != 8192 {
		t.Fatalf("MaxTokens = %d, want 8192", config.LLM.MaxTokens)
	}
	if config.LLM.ThinkingBudgetTokens != 1024 {
		t.Fatalf("ThinkingBudgetTokens = %d, want 1024", config.LLM.ThinkingBudgetTokens)
	}
	if config.LLM.Stream {
		t.Fatal("Stream = true, want false")
	}
}

func TestLoadConfigReadsOpenAICompatibleProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_MODEL=ollama/qwen3:14b",
		"LLM_OLLAMA_FORMAT=openai",
		"LLM_OLLAMA_API_KEY=dummy",
		"LLM_OLLAMA_BASE_URL=http://localhost:11434/v1",
		"LLM_OLLAMA_MAX_TOKENS=2048",
	}, "\n"))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.LLM.Supplier != "ollama" {
		t.Fatalf("Supplier = %q, want ollama", config.LLM.Supplier)
	}
	if config.LLM.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai format", config.LLM.Provider)
	}
	if config.LLM.APIKey != "dummy" {
		t.Fatalf("APIKey = %q, want dummy", config.LLM.APIKey)
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

func TestLoadConfigUsesModelRefModelOverLegacyProviderModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_MODEL=openrouter/openrouter/owl-alpha",
		"LLM_OPENROUTER_FORMAT=openai",
		"LLM_OPENROUTER_API_KEY=provider-key",
		"LLM_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1",
		"LLM_OPENROUTER_MODEL=legacy-should-not-win",
		"LLM_OPENROUTER_MAX_TOKENS=2048",
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
	if config.LLM.Model != "openrouter/owl-alpha" {
		t.Fatalf("Model = %q", config.LLM.Model)
	}
}

func TestLoadConfigOptionsOverrideModelRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_MODEL=anthropic/claude-test",
		"LLM_OPENROUTER_FORMAT=openai",
		"LLM_OPENROUTER_API_KEY=router-key",
		"LLM_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1",
		"LLM_ANTHROPIC_FORMAT=claude",
		"LLM_ANTHROPIC_API_KEY=anthropic-key",
	}, "\n"))

	config, err := LoadConfigWithOptions(LoadConfigOptions{ModelRef: "openrouter/openrouter/owl-alpha"})
	if err != nil {
		t.Fatalf("LoadConfigWithOptions() error = %v", err)
	}
	if config.LLM.Supplier != "openrouter" {
		t.Fatalf("Supplier = %q, want openrouter", config.LLM.Supplier)
	}
	if config.LLM.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", config.LLM.Provider)
	}
	if config.LLM.Model != "openrouter/owl-alpha" {
		t.Fatalf("Model = %q, want openrouter/owl-alpha", config.LLM.Model)
	}
}

func TestLoadConfigRejectsInvalidModelRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, "LLM_MODEL=owl-alpha\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want model ref error")
	}
	if !strings.Contains(err.Error(), "provider/model") {
		t.Fatalf("LoadConfig() error = %v, want provider/model hint", err)
	}
}

func TestLoadConfigOptionsOverrideFormatAndModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, strings.Join([]string{
		"LLM_MODEL=openrouter/openrouter/owl-alpha",
		"LLM_OPENROUTER_FORMAT=openai",
		"LLM_OPENROUTER_API_KEY=router-key",
		"LLM_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1",
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

	writeConfig(t, home, "LLM_MODEL=broken/qwen3:14b\n")

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
		"LLM_MODEL=claude/claude-test",
		"LLM_CLAUDE_FORMAT=claude",
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
		"LLM_MODEL=claude/claude-test",
		"LLM_CLAUDE_FORMAT=claude",
		"LLM_CLAUDE_API_KEY=test-key",
		"LLM_OPENAI_FORMAT=openai",
		"LLM_OPENAI_BASE_URL=",
	}, "\n"))

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestLoadConfigRejectsMissingModelRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeConfig(t, home, "LLM_OPENAI_FORMAT=openai\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want model error")
	}
	if !strings.Contains(err.Error(), "LLM_MODEL") {
		t.Fatalf("LoadConfig() error = %v, want LLM_MODEL", err)
	}
}

func TestLoadConfigRejectsClaudeWithoutAPIKey(t *testing.T) {
	config := defaultConfig()
	config.LLM.Supplier = "claude"
	config.LLM.Provider = "claude"
	config.LLM.Model = "claude-test"
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
		"LLM_MODEL=openai/qwen2.5-coder:7b",
		"LLM_OPENAI_FORMAT=OPENAI",
		"LLM_OPENAI_BASE_URL=http://localhost:11434/v1",
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

func writePrompt(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
