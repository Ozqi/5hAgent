// edit.go - 文件编辑工具
// 功能：精确字符串替换（old_string -> new_string），支持全部替换
// 主要类型：EditInput, EditOutput
// 导出函数：NewEditTool
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: edit
// Desc: 对文件做精确字符串替换。所有匹配的 old_string 都会被替换为 new_string。
//
//	不会重写整个文件，适合小段修改。
//
// Input Parameters:
//   - path       (string, required)  : 文件绝对路径
//   - old_string (string, required)  : 待替换的确切字符串（必须完全匹配）
//   - new_string (string, required)  : 替换后的字符串
//
// Error Scenarios (LLM Hints):
//   - MISSING 'path'       → 必须提供文件绝对路径
//   - MISSING 'old_string' → 必须提供待替换的确切字符串（用 read_file 确认原文）
//   - MISSING 'new_string' → 必须提供替换内容，new_string 不能为空
//   - old_string not found → old_string 与文件内容不匹配；请重新 read_file 确认原文
//     常见原因：空格/缩进/注释差异，用 read_file 核对精确内容
//   - permission denied    → 无写入权限；检查文件权限
//
// Tips:
//   - old_string 必须是精确匹配，包括空格和缩进
//   - 建议先 read_file 确认原文再 edit
//   - 所有匹配的 old_string 都会被替换（批量操作）
//   - 写入大段/新文件内容用 write_file
//
// ============================================================
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
	editToolErrors = `MISSING 'path': 必须提供文件绝对路径
MISSING 'old_string': 必须提供待替换的确切字符串（用 read_file 确认原文）
MISSING 'new_string': new_string 不能为空
old_string not found: old_string 与文件内容不匹配；用 read_file 核对精确内容（包括空格/缩进）
permission denied: 无写入权限；检查文件权限`
	editToolTips = `old_string 必须是精确匹配，包括空格和缩进
建议先 read_file 确认原文再 edit
所有匹配的 old_string 都会被替换（批量操作）
写入大段/新文件内容用 write_file`
)

// EditInput defines the input parameters for edit tool
type EditInput struct {
	Path      string `json:"path" jsonschema:"required,description=Required absolute path to the file to edit"`
	OldString string `json:"old_string" jsonschema:"required,description=Required exact string to replace. Must match exactly, including whitespace and indentation. Use read_file first."`
	NewString string `json:"new_string" jsonschema:"required,description=Required replacement string"`
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
		editToolName,
		editToolDesc,
		func(ctx context.Context, input EditInput) (*schema.ToolResult, error) {
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the absolute file path (e.g., '/home/user/project/file.py')")
			}
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

			if !strings.Contains(originalContent, input.OldString) {
				return nil, fmt.Errorf("old_string not found in '%s'. The string you provided does not match any part of the file. Use read_file to confirm the exact text, including spaces and indentation.", input.Path)
			}

			newContent := strings.ReplaceAll(originalContent, input.OldString, input.NewString)
			if err := os.WriteFile(input.Path, []byte(newContent), 0644); err != nil {
				return nil, fmt.Errorf("failed to write file: %w. Check file permissions.", err)
			}
			return JSONResult(EditOutput{Success: true, Message: fmt.Sprintf("Replaced %d occurrence(s)", strings.Count(originalContent, input.OldString)), Replacements: strings.Count(originalContent, input.OldString)})
		},
	)
}
