// glob.go - 文件模式匹配工具
// 功能：支持 * 和 ** 通配符，递归/非递归搜索
// 主要类型：GlobInput, GlobOutput
// 导出函数：NewGlobTool, recursiveGlob
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// GlobInput defines the input parameters for glob tool
type GlobInput struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Glob pattern to match files (e.g. '*.go' or 'internal/**/*.go')"`
	Path    string `json:"path,omitempty" jsonschema:"description=Base directory to search in (default: current directory)"`
}

// GlobOutput defines the output structure for glob tool
type GlobOutput struct {
	Files []string `json:"files"`
	Count int      `json:"count"`
}

// NewGlobTool creates a new glob tool for file pattern matching
func NewGlobTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"base.glob",
		"Find files matching a glob pattern. Supports wildcards: * (any chars), ** (recursive dirs), ? (single char). Returns sorted list of matching file paths.",
		func(ctx context.Context, input GlobInput) (*schema.ToolResult, error) {
			// Set default path
			if input.Path == "" {
				input.Path = "."
			}

			// Build full pattern
			fullPattern := filepath.Join(input.Path, input.Pattern)

			// Use filepath.Glob for simple patterns (no **)
			var matches []string
			var err error

			if strings.Contains(input.Pattern, "**") {
				// Handle recursive glob with **
				matches, err = recursiveGlob(input.Path, input.Pattern)
			} else {
				// Simple glob
				matches, err = filepath.Glob(fullPattern)
			}

			if err != nil {
				return nil, fmt.Errorf("glob pattern error: %w", err)
			}

			// Sort results
			sort.Strings(matches)
			return JSONResult(GlobOutput{Files: matches, Count: len(matches)})
		},
	)
}

// recursiveGlob handles patterns with ** for recursive directory matching
func recursiveGlob(basePath, pattern string) ([]string, error) {
	var matches []string

	// Split pattern by **
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ** pattern: %s", pattern)
	}

	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	// Walk directory tree
	err := filepath.Walk(filepath.Join(basePath, prefix), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Match suffix pattern
		if suffix != "" {
			matched, err := filepath.Match(suffix, filepath.Base(path))
			if err != nil {
				return nil
			}
			if matched {
				matches = append(matches, path)
			}
		} else {
			// No suffix, match all
			if !info.IsDir() {
				matches = append(matches, path)
			}
		}

		return nil
	})

	return matches, err
}
