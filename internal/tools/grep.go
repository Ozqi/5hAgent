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
)

// GrepInput 描述 grep 工具的输入参数
type GrepInput struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Required regex pattern to search for. Escape metacharacters for literal text."`
	Path    string `json:"path,omitempty" jsonschema:"description=Optional directory or file to search in. Default: current workspace."`
	Type    string `json:"type,omitempty" jsonschema:"description=Optional ripgrep file type filter, e.g. go, py, js, md."`
}

// GrepMatch 描述一条搜索匹配结果
type GrepMatch struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

// GrepOutput 描述 grep 工具的输出结构
type GrepOutput struct {
	Matches []GrepMatch `json:"matches"`
	Count   int         `json:"count"`
}

// NewGrepTool 创建用于代码搜索的 grep 工具
func NewGrepTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		grepToolName,
		grepToolDesc,
		func(ctx context.Context, input GrepInput) (*schema.ToolResult, error) {
			// pattern、type 和 path 均来自模型；通过 argv 调用外部程序，不经过 shell 展开。
			if _, err := exec.LookPath("rg"); err != nil {
				return grepFallback(ctx, input, root)
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

			args = append(args, resolvePath(root, input.Path))

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

// parseRipgrepJSON 解析 ripgrep 的 JSON 输出
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

// grepFallback 在 ripgrep 不可用时使用标准 grep
func grepFallback(ctx context.Context, input GrepInput, root string) (*schema.ToolResult, error) {
	// fallback 同样使用 argv 传参；resolvePath 不限制搜索范围必须位于 workspace 内。
	path := resolvePath(root, input.Path)

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
