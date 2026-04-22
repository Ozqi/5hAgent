package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader(t *testing.T) {
	// 创建临时测试目录
	tmpDir := t.TempDir()

	// 创建测试提示词文件
	testPrompt := `---
name: test_prompt
description: A test prompt
version: "1.0"
type: system
variables:
  var1: "default1"
  var2: "default2"
---

This is a test prompt with {{var1}} and {{var2}}.`

	promptPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(promptPath, []byte(testPrompt), 0644); err != nil {
		t.Fatalf("Failed to create test prompt file: %v", err)
	}

	// 测试加载器
	loader := NewLoader(tmpDir)
	if err := loader.Load(); err != nil {
		t.Fatalf("Failed to load prompts: %v", err)
	}

	// 测试 Get
	prompt, ok := loader.Get("test_prompt")
	if !ok {
		t.Fatal("Prompt not found")
	}

	if prompt.Name != "test_prompt" {
		t.Errorf("Expected name 'test_prompt', got '%s'", prompt.Name)
	}

	if prompt.Type != "system" {
		t.Errorf("Expected type 'system', got '%s'", prompt.Type)
	}

	// 测试 GetContent
	content, err := loader.GetContent("test_prompt")
	if err != nil {
		t.Fatalf("Failed to get content: %v", err)
	}

	expected := "This is a test prompt with {{var1}} and {{var2}}."
	if content != expected {
		t.Errorf("Expected content '%s', got '%s'", expected, content)
	}

	// 测试 GetContentWithVars
	vars := map[string]string{
		"var1": "value1",
		"var2": "value2",
	}
	content, err = loader.GetContentWithVars("test_prompt", vars)
	if err != nil {
		t.Fatalf("Failed to get content with vars: %v", err)
	}

	expected = "This is a test prompt with value1 and value2."
	if content != expected {
		t.Errorf("Expected content '%s', got '%s'", expected, content)
	}

	// 测试 List
	prompts := loader.List()
	if len(prompts) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(prompts))
	}
}

func TestLoaderInvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建无效的提示词文件（缺少 frontmatter）
	invalidPrompt := `This is invalid prompt without frontmatter.`
	promptPath := filepath.Join(tmpDir, "invalid.md")
	if err := os.WriteFile(promptPath, []byte(invalidPrompt), 0644); err != nil {
		t.Fatalf("Failed to create invalid prompt file: %v", err)
	}

	loader := NewLoader(tmpDir)
	// Load 应该成功，但会跳过无效文件
	if err := loader.Load(); err != nil {
		t.Fatalf("Load should not fail on invalid files: %v", err)
	}

	// 应该没有加载任何提示词
	prompts := loader.List()
	if len(prompts) != 0 {
		t.Errorf("Expected 0 prompts, got %d", len(prompts))
	}
}

func TestLoaderNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader(tmpDir)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load should succeed on empty directory: %v", err)
	}

	// 测试获取不存在的提示词
	_, err := loader.GetContent("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent prompt")
	}
}
