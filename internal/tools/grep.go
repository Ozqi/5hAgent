// grep.go - 代码搜索工具
// 功能：调用 ripgrep（或 fallback grep）搜索，支持正则和文件类型过滤
// 主要类型：GrepInput, GrepOutput, GrepMatch
// 导出函数：NewGrepTool, parseRipgrepJSON, grepFallback
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: grep
// Desc: 在文件或目录中搜索正则表达式模式，返回匹配行（带文件路径、行号、列号、内容）。
//
//	优先使用 ripgrep (rg)，ripgrep 不可用时 fallback 到 grep。read_only 工具。
//
// Input Parameters:
//   - pattern (string, required)  : 正则表达式搜索模式
//   - path     (string, optional) : 搜索目录或文件，默认当前目录
//   - type     (string, optional) : 按文件类型过滤，如 'go'、'py'、'js'
//
// Error Scenarios (LLM Hints):
//   - pattern invalid             → 正则表达式语法错误；简化模式或转义特殊字符
//   - path not found              → 搜索路径不存在；确认目录/文件名
//   - no matches                  → 无匹配结果（正常情况，非错误）；尝试更宽松的模式
//   - rg not found (fallback)    → 系统未安装 ripgrep，自动使用 grep（功能受限）
//
// Tips:
//   - 正则特殊字符需要转义：. * + ? [ ] ( ) { } | \
//   - 按类型过滤：`type: go` 只搜索 .go 文件
//
// ============================================================
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// --- LLM 描述常量（供 InferEnhancedTool 使用）---
const (
	grepToolName = "base.grep"
	grepToolDesc = `Search file contents with a regular expression and return matching file path, line, column, and text. Always send JSON object arguments.
- pattern: required regex pattern. Escape regex metacharacters when searching literal text.
- path: optional directory or file. Default current workspace.
- type: optional ripgrep file type such as go, py, js, md.
Examples: {"pattern":"func NewAgent","path":"internal","type":"go"}; {"pattern":"claude-context","path":"doc"}`
	grepToolErrors = `pattern invalid: 正则表达式语法错误；简化模式或转义特殊字符
path not found: 搜索路径不存在；确认目录/文件名
no matches: 无匹配结果（正常情况，非错误）；尝试更宽松的模式
rg not found (fallback): 系统未安装 ripgrep，自动使用 grep（功能受限）`
	grepToolTips = `正则特殊字符需要转义：. * + ? [ ] ( ) { } | \\
按类型过滤：type: go 只搜索 .go 文件`
)

// GrepInput defines the input parameters for grep tool
type GrepInput struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Required regex pattern to search for. Escape metacharacters for literal text."`
	Path    string `json:"path,omitempty" jsonschema:"description=Optional directory or file to search in. Default: current workspace."`
	Type    string `json:"type,omitempty" jsonschema:"description=Optional ripgrep file type filter, e.g. go, py, js, md."`
}

// GrepMatch represents a single match result
type GrepMatch struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

// GrepOutput defines the output structure for grep tool
type GrepOutput struct {
	Matches []GrepMatch `json:"matches"`
	Count   int         `json:"count"`
}

// NewGrepTool creates a new grep tool for code searching
func NewGrepTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		grepToolName,
		grepToolDesc,
		func(ctx context.Context, input GrepInput) (*schema.ToolResult, error) {
			if _, err := exec.LookPath("rg"); err != nil {
				return grepFallback(ctx, input)
			}

			args := []string{
				"--json",
				"--no-heading",
				"--line-number",
				"--column",
				"--smart-case",
				"--max-count=100",
			}

			if input.Type != "" {
				args = append(args, "--type", input.Type)
			}

			args = append(args, input.Pattern)

			if input.Path != "" {
				args = append(args, input.Path)
			} else {
				args = append(args, ".")
			}

			cmd := exec.CommandContext(ctx, "rg", args...)
			output, err := cmd.CombinedOutput()

			if err != nil && len(output) == 0 {
				return JSONResult(GrepOutput{Matches: nil, Count: 0})
			}

			matches := parseRipgrepJSON(string(output))
			result := GrepOutput{Matches: matches, Count: len(matches)}
			return JSONResult(result)
		},
	)
}

// parseRipgrepJSON parses ripgrep's JSON output
func parseRipgrepJSON(output string) []GrepMatch {
	var matches []GrepMatch

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry["type"] != "match" {
			continue
		}

		data, ok := entry["data"].(map[string]interface{})
		if !ok {
			continue
		}

		path, _ := data["path"].(map[string]interface{})
		pathText, _ := path["text"].(string)

		lineNum, _ := data["line_number"].(float64)

		submatches, _ := data["submatches"].([]interface{})
		if len(submatches) == 0 {
			continue
		}

		submatch, _ := submatches[0].(map[string]interface{})
		matchText, _ := submatch["match"].(map[string]interface{})
		text, _ := matchText["text"].(string)

		start, _ := submatch["start"].(float64)

		matches = append(matches, GrepMatch{
			File:   pathText,
			Line:   int(lineNum),
			Column: int(start) + 1,
			Text:   text,
		})
	}

	return matches
}

// grepFallback uses standard grep when ripgrep is not available
func grepFallback(ctx context.Context, input GrepInput) (*schema.ToolResult, error) {
	path := input.Path
	if path == "" {
		path = "."
	}

	args := []string{
		"-r",
		"-n",
		"-H",
		"--max-count=100",
		input.Pattern,
		path,
	}

	cmd := exec.CommandContext(ctx, "grep", args...)
	output, err := cmd.CombinedOutput()

	if err != nil && len(output) == 0 {
		return JSONResult(GrepOutput{Matches: nil, Count: 0})
	}

	var matches []GrepMatch
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		var lineNum int
		fmt.Sscanf(parts[1], "%d", &lineNum)
		matches = append(matches, GrepMatch{File: parts[0], Line: lineNum, Column: 0, Text: parts[2]})
	}
	return JSONResult(GrepOutput{Matches: matches, Count: len(matches)})
}
