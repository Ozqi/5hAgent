package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsThinkingBudgetTokens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".5hAgent")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	env := "LLM_API_KEY=test-key\nLLM_THINKING_BUDGET_TOKENS=2048\n"
	if err := os.WriteFile(filepath.Join(configDir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.LLM.ThinkingBudgetTokens != 2048 {
		t.Fatalf("ThinkingBudgetTokens = %d, want 2048", config.LLM.ThinkingBudgetTokens)
	}
}
