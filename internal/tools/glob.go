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

// --- LLM 描述常量（供 InferEnhancedTool 使用）---
const (
	globToolName = "base.glob"
	globToolDesc = `Find files by glob pattern and return sorted paths. Always send JSON object arguments.
- pattern: required glob pattern, e.g. "*.go", "**/*.md", "internal/**/*.go".
- path: optional base directory. Default current workspace. Pattern is evaluated under this path.
Use glob to discover file paths before read_file/edit when exact paths are unknown.`
)

// GlobInput 描述 glob 工具的输入参数
type GlobInput struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Required glob pattern, e.g. '*.go', '**/*.md', 'internal/**/*.go'."`
	Path    string `json:"path,omitempty" jsonschema:"description=Optional base directory to search in. Default: current workspace."`
}

// GlobOutput 描述 glob 工具的输出结构
type GlobOutput struct {
	Files []string `json:"files"`
	Count int      `json:"count"`
}

// NewGlobTool 创建用于文件模式匹配的 glob 工具
func NewGlobTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		globToolName,
		globToolDesc,
		func(ctx context.Context, input GlobInput) (*schema.ToolResult, error) {
			// 模型输入决定遍历起点和模式；路径解析不限制搜索范围必须位于 workspace 内。
			input.Path = resolvePath(root, input.Path)

			fullPattern := filepath.Join(input.Path, input.Pattern)

			var matches []string
			var err error

			if strings.Contains(input.Pattern, "**") {
				matches, err = recursiveGlob(input.Path, input.Pattern)
			} else {
				matches, err = filepath.Glob(fullPattern)
			}

			if err != nil {
				return nil, fmt.Errorf("glob pattern error: %w. Check the pattern syntax.", err)
			}

			sort.Strings(matches)
			return JSONResult(GlobOutput{Files: matches, Count: len(matches)})
		},
	)
}

// recursiveGlob 处理带 ** 的递归目录匹配模式
func recursiveGlob(basePath, pattern string) ([]string, error) {
	var matches []string

	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ** pattern: %s", pattern)
	}

	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	err := filepath.Walk(filepath.Join(basePath, prefix), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if suffix != "" {
			matched, err := filepath.Match(suffix, filepath.Base(path))
			if err != nil {
				return nil
			}
			if matched {
				matches = append(matches, path)
			}
		} else {
			if !info.IsDir() {
				matches = append(matches, path)
			}
		}

		return nil
	})

	return matches, err
}
