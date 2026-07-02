// list_dir.go - 目录列表工具
// 功能：列出目录内容，支持递归；返回文件/目录名、路径、大小
// 主要类型：ListDirInput, ListDirOutput, FileInfo
// 导出函数：NewListDirTool
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
		"base.list_dir",
		"List contents of a directory. Returns file names, paths, types (file/dir), and sizes. Supports recursive listing of subdirectories.",
		func(ctx context.Context, input ListDirInput) (*schema.ToolResult, error) {
			// Set default path
			if input.Path == "" {
				input.Path = "."
			}

			// Check if path exists
			info, err := os.Stat(input.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to access path: %w", err)
			}

			if !info.IsDir() {
				return nil, fmt.Errorf("path is not a directory: %s", input.Path)
			}

			var files []FileInfo

			if input.Recursive {
				// Recursive listing
				err = filepath.Walk(input.Path, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil // Skip errors
					}

					// Skip the root directory itself
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
					return nil, fmt.Errorf("failed to walk directory: %w", err)
				}
			} else {
				// Non-recursive listing
				entries, err := os.ReadDir(input.Path)
				if err != nil {
					return nil, fmt.Errorf("failed to read directory: %w", err)
				}

				for _, entry := range entries {
					info, err := entry.Info()
					if err != nil {
						continue // Skip entries we can't stat
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

			// Sort by path
			sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
			return JSONResult(ListDirOutput{Files: files, Count: len(files)})
		},
	)
}
