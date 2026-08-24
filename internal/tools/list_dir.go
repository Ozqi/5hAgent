package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// --- LLM 描述常量（供 InferEnhancedTool 使用）---
const (
	listDirToolName = "base.list_dir"
	listDirToolDesc = `List directory entries and return name, path, directory flag, and size. Always send JSON object arguments.
- path: optional directory path. Default current workspace. Must be a directory, not a file.
- recursive: optional boolean. Default false. Use true only when you need a full tree.
Example: {"path":"internal/tools","recursive":false}`
)

// ListDirInput 描述 list_dir 工具的输入参数
type ListDirInput struct {
	Path      string `json:"path,omitempty" jsonschema:"description=Optional directory path to list. Default: current workspace. Must be a directory."`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"description=Optional. List subdirectories recursively. Default: false."`
}

// FileInfo 描述文件或目录信息
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// ListDirOutput 描述 list_dir 工具的输出结构
type ListDirOutput struct {
	Files []FileInfo `json:"files"`
	Count int        `json:"count"`
}

// NewListDirTool 创建用于列出目录内容的 list_dir 工具
func NewListDirTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		listDirToolName,
		listDirToolDesc,
		func(ctx context.Context, input ListDirInput) (*schema.ToolResult, error) {
			// 模型输入在这里进入文件系统信任边界；resolvePath 只拼接相对路径。
			input.Path = resolvePath(root, input.Path)

			info, err := os.Stat(input.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to access path '%s': %w. Verify the path exists and is accessible.", input.Path, err)
			}

			if !info.IsDir() {
				return nil, fmt.Errorf("path '%s' is not a directory. Use read_file to read file content.", input.Path)
			}

			var files []FileInfo

			if input.Recursive {
				err = filepath.Walk(input.Path, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil
					}

					if path == input.Path {
						return nil
					}

					files = append(files, FileInfo{
						Name:  info.Name(),
						Path:  path,
						IsDir: info.IsDir(),
						Size:  info.Size(),
					})

					return nil
				})

				if err != nil {
					return nil, fmt.Errorf("failed to walk directory: %w. Some subdirectories may be inaccessible.", err)
				}
			} else {
				entries, err := os.ReadDir(input.Path)
				if err != nil {
					return nil, fmt.Errorf("failed to read directory: %w. Check directory permissions.", err)
				}

				for _, entry := range entries {
					info, err := entry.Info()
					if err != nil {
						continue
					}

					fullPath := filepath.Join(input.Path, entry.Name())
					files = append(files, FileInfo{
						Name:  entry.Name(),
						Path:  fullPath,
						IsDir: entry.IsDir(),
						Size:  info.Size(),
					})
				}
			}

			sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
			return JSONResult(ListDirOutput{Files: files, Count: len(files)})
		},
	)
}
