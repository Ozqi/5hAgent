// list_dir.go - 目录列表工具
// 功能：列出目录内容，支持递归；返回文件/目录名、路径、大小
// 主要类型：ListDirInput, ListDirOutput, FileInfo
// 导出函数：NewListDirTool
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: list_dir
// Desc: 列出目录内容，返回文件/目录的名称、路径、类型、大小。
//
//	支持递归模式。read_only 工具。
//
// Input Parameters:
//   - path       (string, optional) : 目录路径，默认当前目录
//   - recursive  (bool,   optional) : 是否递归列出子目录，默认 false
//
// Error Scenarios (LLM Hints):
//   - path not found              → 目录不存在；确认路径是否正确
//   - path is not a directory      → 指定路径是文件而非目录；用 read_file 读取
//   - permission denied           → 无读取权限
//   - empty directory              → 目录为空（正常情况，非错误）
//
// Tips:
//   - 非递归模式适合快速浏览当前目录结构
//   - recursive=true 会列出所有子目录内容，适合了解项目全貌
//
// ============================================================
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
	listDirToolDesc = `列出目录内容，返回文件/目录的名称、路径、类型、大小。支持递归模式。
- path: 目录路径，默认当前目录（可选）
- recursive: 是否递归列出子目录，默认 false（可选）`
	listDirToolErrors = `path not found: 目录不存在；确认路径是否正确
path is not a directory: 指定路径是文件而非目录；用 read_file 读取
permission denied: 无读取权限
empty directory: 目录为空（正常情况，非错误）`
	listDirToolTips = `非递归模式适合快速浏览当前目录结构
recursive=true 会列出所有子目录内容，适合了解项目全貌`
)

// ListDirInput defines the input parameters for list_dir tool
type ListDirInput struct {
	Path      string `json:"path,omitempty" jsonschema:"description=Directory path to list (default: current directory)"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"description=List subdirectories recursively (default: false)"`
}

// FileInfo represents information about a file or directory
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// ListDirOutput defines the output structure for list_dir tool
type ListDirOutput struct {
	Files []FileInfo `json:"files"`
	Count int        `json:"count"`
}

// NewListDirTool creates a new list_dir tool for listing directory contents
func NewListDirTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		listDirToolName,
		listDirToolDesc,
		func(ctx context.Context, input ListDirInput) (*schema.ToolResult, error) {
			if input.Path == "" {
				input.Path = "."
			}

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
