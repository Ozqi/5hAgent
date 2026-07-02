// edit.go - 文件编辑工具
// 功能：精确字符串替换（old_string -> new_string），支持全部替换
// 主要类型：EditInput, EditOutput
// 导出函数：NewEditTool
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// EditInput defines the input parameters for edit tool
type EditInput struct {
	Path      string `json:"path" jsonschema:"required,description=Absolute path to the file to edit"`
	OldString string `json:"old_string" jsonschema:"required,description=Exact string to replace (must match exactly)"`
	NewString string `json:"new_string" jsonschema:"required,description=New string to replace with"`
}

// EditOutput defines the output structure for edit tool
type EditOutput struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	Replacements int    `json:"replacements"`
}

// NewEditTool creates a new edit tool for precise file editing
func NewEditTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"base.edit",
		"Edit a file by replacing exact string matches. REQUIRED: path (absolute file path), old_string (exact match), new_string (replacement). Returns the number of replacements made.",
		func(ctx context.Context, input EditInput) (*schema.ToolResult, error) {
			// Validate required parameters
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the absolute file path (e.g., '/home/user/project/file.py')")
			}
			if input.OldString == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'old_string' is required. You must provide the exact string to replace")
			}
			if input.NewString == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'new_string' is required. You must provide the replacement string")
			}

			// Read file content
			content, err := os.ReadFile(input.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to read file '%s': %w. Make sure the path is correct and the file exists. Use absolute paths like '/home/user/project/file.py'", input.Path, err)
			}

			originalContent := string(content)

			// Check if old_string exists
			if !strings.Contains(originalContent, input.OldString) {
				return JSONResult(EditOutput{Success: false, Message: fmt.Sprintf("old_string not found in %s", input.Path), Replacements: 0})
			}

			// Replace all occurrences
			newContent := strings.ReplaceAll(originalContent, input.OldString, input.NewString)
			if err := os.WriteFile(input.Path, []byte(newContent), 0644); err != nil {
				return nil, fmt.Errorf("failed to write file: %w", err)
			}
			return JSONResult(EditOutput{Success: true, Message: fmt.Sprintf("Replaced %d occurrence(s)", strings.Count(originalContent, input.OldString)), Replacements: strings.Count(originalContent, input.OldString)})
		},
	)
}
