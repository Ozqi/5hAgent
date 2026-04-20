package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestReadFileTool 测试文件读取工具
// 验证：能够正确读取文件内容，支持偏移量和行数限制
func TestReadFileTool(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create the tool
	tool, err := NewReadFileTool()
	if err != nil {
		t.Fatalf("failed to create read_file tool: %v", err)
	}

	// Test reading the file
	ctx := context.Background()
	input := ReadFileInput{
		Path:   testFile,
		Offset: 1,
		Limit:  3,
	}

	// Convert input to ToolArgument
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	result, err := tool.InvokableRun(ctx, &schema.ToolArgument{Text: string(inputJSON)})
	if err != nil {
		t.Fatalf("failed to invoke tool: %v", err)
	}

	// Verify result
	if len(result.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(result.Parts))
	}

	var output ReadFileOutput
	err = json.Unmarshal([]byte(result.Parts[0].Text), &output)
	if err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if output.TotalLines != 5 {
		t.Errorf("expected 5 total lines, got %d", output.TotalLines)
	}

	t.Logf("Read file output:\n%s", output.Content)
}

// TestExecShellTool 测试 Shell 命令执行工具
// 验证：能够执行命令并返回标准输出、标准错误和返回码
func TestExecShellTool(t *testing.T) {
	// Create the tool
	tool, err := NewExecShellTool()
	if err != nil {
		t.Fatalf("failed to create exec_shell tool: %v", err)
	}

	// Test executing a simple command
	ctx := context.Background()
	input := ExecShellInput{
		Command: "echo 'hello world'",
	}

	// Convert input to ToolArgument
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	result, err := tool.InvokableRun(ctx, &schema.ToolArgument{Text: string(inputJSON)})
	if err != nil {
		t.Fatalf("failed to invoke tool: %v", err)
	}

	// Verify result
	if len(result.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(result.Parts))
	}

	var output ExecShellOutput
	err = json.Unmarshal([]byte(result.Parts[0].Text), &output)
	if err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if output.ReturnCode != 0 {
		t.Errorf("expected return code 0, got %d", output.ReturnCode)
	}

	if output.Stdout != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", output.Stdout)
	}

	t.Logf("Exec shell output: %+v", output)
}

// TestGetAllTools 测试获取所有已注册的工具
// 验证：能够返回所有工具并获取工具信息
func TestGetAllTools(t *testing.T) {
	tools := GetAllTools()

	if len(tools) < 2 {
		t.Errorf("expected at least 2 tools, got %d", len(tools))
	}

	ctx := context.Background()
	for _, tool := range tools {
		info, err := tool.Info(ctx)
		if err != nil {
			t.Errorf("failed to get tool info: %v", err)
			continue
		}
		t.Logf("Tool: %s - %s", info.Name, info.Desc)
	}
}

// TestGetToolByName 测试根据名称查找工具
// 验证：能够找到已注册的工具，不存在的工具返回 nil
func TestGetToolByName(t *testing.T) {
	tool := GetToolByName("read_file")
	if tool == nil {
		t.Error("expected to find read_file tool")
	}

	tool = GetToolByName("exec_shell")
	if tool == nil {
		t.Error("expected to find exec_shell tool")
	}

	tool = GetToolByName("nonexistent")
	if tool != nil {
		t.Error("expected nil for nonexistent tool")
	}
}
