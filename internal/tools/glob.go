// glob.go - 文件模式匹配工具
// 功能：支持 * 和 ** 通配符，递归/非递归搜索
// 主要类型：GlobInput, GlobOutput
// 导出函数：NewGlobTool, recursiveGlob
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: glob
// Desc: 按 glob 模式匹配文件路径，返回排序后的匹配文件列表。
//
//	支持通配符：* 任意字符、** 递归目录、? 单字符。read_only 工具。
//
// Input Parameters:
//   - pattern (string, required)  : glob 模式，如 '*.go'、'**/*.md'、'src/**/*.ts'
//   - path     (string, optional) : 搜索起始目录，默认当前目录
//
// Error Scenarios (LLM Hints):
//   - pattern invalid             → glob 模式语法错误；常见错误：多余的 **、不匹配的引号
//   - path not found              → 起始目录不存在；确认目录路径
//   - permission denied           → 无目录读取权限
//   - no matches                  → 无匹配结果（正常情况，非错误）
//
// Tips:
//   - '**' 递归搜索子目录（慎用，避免返回过多文件）
//   - '*' 在单层目录内匹配
//   - 常用模式：'*.go'、'**/*.go'、'**/test_*.py'、'**/node_modules/**'
//
// ============================================================
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
	globToolErrors = `pattern invalid: glob 模式语法错误；常见错误：多余的 **、不匹配的引号
path not found: 起始目录不存在；确认目录路径
permission denied: 无目录读取权限
no matches: 无匹配结果（正常情况，非错误）`
	globToolTips = `'**' 递归搜索子目录（慎用，避免返回过多文件）
'*' 在单层目录内匹配
常用模式：'*.go'、'**/*.go'、'**/test_*.py'、'**/node_modules/**'`
)

// GlobInput defines the input parameters for glob tool
type GlobInput struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Required glob pattern, e.g. '*.go', '**/*.md', 'internal/**/*.go'."`
	Path    string `json:"path,omitempty" jsonschema:"description=Optional base directory to search in. Default: current workspace."`
}

// GlobOutput defines the output structure for glob tool
type GlobOutput struct {
	Files []string `json:"files"`
	Count int      `json:"count"`
}

// NewGlobTool creates a new glob tool for file pattern matching
func NewGlobTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		globToolName,
		globToolDesc,
		func(ctx context.Context, input GlobInput) (*schema.ToolResult, error) {
			if input.Path == "" {
				input.Path = "."
			}

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

// recursiveGlob handles patterns with ** for recursive directory matching
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
