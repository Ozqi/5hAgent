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

// --- LLM 描述常量（供 InferEnhancedTool 使用）---
const (
	editToolName = "base.edit"
	editToolDesc = `Precise string replacement in an existing file. Always send JSON object arguments.
- path: required absolute file path.
- old_string: required exact text to replace. Must match file content exactly, including whitespace and indentation. Use read_file first.
- new_string: required replacement text. All occurrences of old_string are replaced.
Example: {"path":"/home/user/project/main.go","old_string":"old exact text","new_string":"new exact text"}`
)

// EditInput 描述 edit 工具的输入参数
type EditInput struct {
	Path      string `json:"path" jsonschema:"required,description=Required absolute path to the file to edit"`
	OldString string `json:"old_string" jsonschema:"required,description=Required exact string to replace. Must match exactly, including whitespace and indentation. Use read_file first."`
	NewString string `json:"new_string" jsonschema:"required,description=Required replacement string"`
}

// EditOutput 描述 edit 工具的输出结构
type EditOutput struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	Replacements int    `json:"replacements"`
}

// NewEditTool 创建用于精确文件编辑的 edit 工具
func NewEditTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		editToolName,
		editToolDesc,
		func(ctx context.Context, input EditInput) (*schema.ToolResult, error) {
			// 模型输入在这里进入文件系统信任边界；路径解析不提供 workspace confinement。
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the absolute file path (e.g., '/home/user/project/file.py')")
			}
			input.Path = resolvePath(root, input.Path)
			if input.OldString == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'old_string' is required. You must provide the exact string to replace. Use read_file first to confirm the exact text.")
			}
			if input.NewString == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'new_string' is required. You must provide the replacement string")
			}

			content, err := os.ReadFile(input.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to read file '%s': %w. Make sure the path is correct and the file exists. Use absolute paths like '/home/user/project/file.py'", input.Path, err)
			}

			originalContent := string(content)
			replacements := strings.Count(originalContent, input.OldString)
			if replacements == 0 {
				return nil, fmt.Errorf("old_string not found in '%s'. The string you provided does not match any part of the file. Use read_file to confirm the exact text, including spaces and indentation.", input.Path)
			}

			// 在内存中完成全部替换后直接覆盖原文件；该写入不是原子替换。
			newContent := strings.ReplaceAll(originalContent, input.OldString, input.NewString)
			if err := os.WriteFile(input.Path, []byte(newContent), 0644); err != nil {
				return nil, fmt.Errorf("failed to write file: %w. Check file permissions.", err)
			}
			return JSONResult(EditOutput{Success: true, Message: fmt.Sprintf("Replaced %d occurrence(s)", replacements), Replacements: replacements})
		},
	)
}
