// write_file.go - 文件写入工具
// 功能：创建或覆盖文件，自动创建父目录
// 主要类型：WriteFileInput, WriteFileOutput
// 导出函数：NewWriteFileTool
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// WriteFileInput defines the input parameters for write_file tool
type WriteFileInput struct {
	Path    string `json:"path" jsonschema:"required,description=Absolute path to the file to write"`
	Content string `json:"content" jsonschema:"required,description=Content to write to the file"`
}

// WriteFileOutput defines the output structure for write_file tool
type WriteFileOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Bytes   int    `json:"bytes"`
}

// NewWriteFileTool creates a new write_file tool using Eino's InferEnhancedTool
func NewWriteFileTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"base.write_file",
		"Write non-empty content to a file. Always send JSON object arguments. Required: path (absolute file path), content (full file content). Creates parent directories if needed and overwrites existing file. Example: {\"path\":\"/home/user/project/file.txt\",\"content\":\"hello\\n\"}",
		func(ctx context.Context, input WriteFileInput) (*schema.ToolResult, error) {
			// Validate input
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the file path to write")
			}
			if input.Content == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'content' is required. You must provide the content to write")
			}

			// Create parent directories if they don't exist
			dir := filepath.Dir(input.Path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create parent directories for '%s': %w", input.Path, err)
			}

			// Write file
			if err := os.WriteFile(input.Path, []byte(input.Content), 0644); err != nil {
				return nil, fmt.Errorf("failed to write: %w", err)
			}
			return JSONResult(WriteFileOutput{Success: true, Message: fmt.Sprintf("Written to %s", input.Path), Bytes: len(input.Content)})
		},
	)
}
