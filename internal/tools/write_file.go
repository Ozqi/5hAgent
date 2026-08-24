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

// WriteFileInput 描述 write_file 工具的输入参数
type WriteFileInput struct {
	Path    string `json:"path" jsonschema:"required,description=Absolute path to the file to write"`
	Content string `json:"content" jsonschema:"required,description=Content to write to the file"`
}

// WriteFileOutput 描述 write_file 工具的输出结构
type WriteFileOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Bytes   int    `json:"bytes"`
}

// NewWriteFileTool 使用 Eino InferEnhancedTool 创建 write_file 工具
func NewWriteFileTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		"base.write_file",
		"Write non-empty content to a file. Always send JSON object arguments. Required: path (absolute file path), content (full file content). Creates parent directories if needed and overwrites existing file. Example: {\"path\":\"/home/user/project/file.txt\",\"content\":\"hello\\n\"}",
		func(ctx context.Context, input WriteFileInput) (*schema.ToolResult, error) {
			// 模型输入在这里进入文件系统信任边界；resolvePath 只解析相对路径，不限制绝对路径或上级目录。
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the file path to write")
			}
			input.Path = resolvePath(root, input.Path)
			if input.Content == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'content' is required. You must provide the content to write")
			}

			// 先创建父目录，再以固定权限覆盖目标文件；任一步失败都直接返回。
			dir := filepath.Dir(input.Path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create parent directories for '%s': %w", input.Path, err)
			}

			if err := os.WriteFile(input.Path, []byte(input.Content), 0644); err != nil {
				return nil, fmt.Errorf("failed to write: %w", err)
			}
			return JSONResult(WriteFileOutput{Success: true, Message: fmt.Sprintf("Written to %s", input.Path), Bytes: len(input.Content)})
		},
	)
}
