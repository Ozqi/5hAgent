package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfigDir(t *testing.T) {
	dir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir failed: %v", err)
	}
	if dir == "" {
		t.Error("GetConfigDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("GetConfigDir returned relative path: %s", dir)
	}
}

func TestGetConfigPath(t *testing.T) {
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath failed: %v", err)
	}
	if path == "" {
		t.Error("GetConfigPath returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("GetConfigPath returned relative path: %s", path)
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("GetConfigPath returned wrong filename: %s", filepath.Base(path))
	}
}

func TestDefaultConfig(t *testing.T) {
	config := defaultConfig()

	if config.LLM.BaseURL != DefaultBaseURL {
		t.Errorf("Expected BaseURL %s, got %s", DefaultBaseURL, config.LLM.BaseURL)
	}
	if config.LLM.Model != DefaultModel {
		t.Errorf("Expected Model %s, got %s", DefaultModel, config.LLM.Model)
	}
	if config.LLM.MaxTokens != DefaultMaxTokens {
		t.Errorf("Expected MaxTokens %d, got %d", DefaultMaxTokens, config.LLM.MaxTokens)
	}
	if config.Agent.Name != DefaultAgentName {
		t.Errorf("Expected AgentName %s, got %s", DefaultAgentName, config.Agent.Name)
	}
	if config.Agent.MaxTotalTokens != DefaultMaxTotalTokens {
		t.Errorf("Expected MaxTotalTokens %d, got %d", DefaultMaxTotalTokens, config.Agent.MaxTotalTokens)
	}
	if config.Agent.RepeatToolLimit != DefaultRepeatToolLimit {
		t.Errorf("Expected RepeatToolLimit %d, got %d", DefaultRepeatToolLimit, config.Agent.RepeatToolLimit)
	}
}

func TestValidate_MissingAPIKey(t *testing.T) {
	config := defaultConfig()
	config.LLM.APIKey = ""

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for missing API key")
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	config := defaultConfig()
	config.LLM.APIKey = "test-key"

	err := config.Validate()
	if err != nil {
		t.Errorf("Validation failed for valid config: %v", err)
	}
}

func TestValidate_InvalidMaxTokens(t *testing.T) {
	config := defaultConfig()
	config.LLM.APIKey = "test-key"
	config.LLM.MaxTokens = -1

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for negative MaxTokens")
	}
}

func TestLoadFromEnv(t *testing.T) {
	// 设置环境变量
	os.Setenv("CLAUDE_API_KEY", "test-api-key")
	os.Setenv("CLAUDE_BASE_URL", "https://test.example.com")
	os.Setenv("CLAUDE_MODEL", "test-model")
	defer func() {
		os.Unsetenv("CLAUDE_API_KEY")
		os.Unsetenv("CLAUDE_BASE_URL")
		os.Unsetenv("CLAUDE_MODEL")
	}()

	config := defaultConfig()
	err := loadFromEnv(config)
	if err != nil {
		t.Fatalf("loadFromEnv failed: %v", err)
	}

	if config.LLM.APIKey != "test-api-key" {
		t.Errorf("Expected APIKey 'test-api-key', got '%s'", config.LLM.APIKey)
	}
	if config.LLM.BaseURL != "https://test.example.com" {
		t.Errorf("Expected BaseURL 'https://test.example.com', got '%s'", config.LLM.BaseURL)
	}
	if config.LLM.Model != "test-model" {
		t.Errorf("Expected Model 'test-model', got '%s'", config.LLM.Model)
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	err := createDefaultConfig(configPath)
	if err != nil {
		t.Fatalf("createDefaultConfig failed: %v", err)
	}

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// 读取文件内容
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// 检查内容
	contentStr := string(content)
	if len(contentStr) == 0 {
		t.Error("Config file is empty")
	}

	// 检查是否包含关键字段
	expectedStrings := []string{
		"llm:",
		"api_key:",
		"agent:",
		"mcp:",
		"servers:",
	}
	for _, expected := range expectedStrings {
		if !contains(contentStr, expected) {
			t.Errorf("Config file missing expected string: %s", expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
