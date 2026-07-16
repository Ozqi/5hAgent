// read_md.go - Markdown 结构化读取工具
// 功能：列出 Markdown 标题树，或读取指定标题下的 section。
package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

const (
	readMDToolName = "base.read_md"
	readMDToolDesc = `Read Markdown structurally. Always send JSON object arguments.
- action: required, one of list_headings or read_section.
- path: required Markdown file path.
- heading: required for read_section. Match heading text exactly, without leading #.
Use list_headings before read_section when unsure about the exact heading. This tool is read-only.`
)

type ReadMDInput struct {
	Action  string `json:"action" jsonschema:"required,description=Action to run: list_headings or read_section."`
	Path    string `json:"path" jsonschema:"required,description=Required Markdown file path."`
	Heading string `json:"heading,omitempty" jsonschema:"description=Required for read_section. Exact heading text without leading #."`
}

type MarkdownHeading struct {
	Level int    `json:"level"`
	Title string `json:"title"`
	Line  int    `json:"line"`
}

type ReadMDOutput struct {
	Path       string            `json:"path"`
	Action     string            `json:"action"`
	Headings   []MarkdownHeading `json:"headings,omitempty"`
	Heading    string            `json:"heading,omitempty"`
	StartLine  int               `json:"start_line,omitempty"`
	EndLine    int               `json:"end_line,omitempty"`
	Content    string            `json:"content,omitempty"`
	TotalLines int               `json:"total_lines"`
}

var markdownHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)

func NewReadMDTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		readMDToolName,
		readMDToolDesc,
		func(ctx context.Context, input ReadMDInput) (*schema.ToolResult, error) {
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required")
			}
			input.Path = resolvePath(root, input.Path)
			lines, headings, err := readMarkdownLines(input.Path)
			if err != nil {
				return nil, err
			}
			switch input.Action {
			case "list_headings":
				return JSONResult(ReadMDOutput{Path: input.Path, Action: input.Action, Headings: headings, TotalLines: len(lines)})
			case "read_section":
				if strings.TrimSpace(input.Heading) == "" {
					return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'heading' is required for read_section")
				}
				output, err := markdownSection(input.Path, lines, headings, input.Heading)
				if err != nil {
					return nil, err
				}
				return JSONResult(output)
			default:
				return nil, fmt.Errorf("unknown action %q: expected list_headings or read_section", input.Action)
			}
		},
	)
}

func readMarkdownLines(path string) ([]string, []MarkdownHeading, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open Markdown file %q: %w", path, err)
	}
	defer file.Close()

	var lines []string
	var headings []MarkdownHeading
	inFence := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if inFence {
			continue
		}
		if match := markdownHeadingPattern.FindStringSubmatch(line); match != nil {
			headings = append(headings, MarkdownHeading{
				Level: len(match[1]),
				Title: strings.TrimSpace(match[2]),
				Line:  len(lines),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read Markdown file %q: %w", path, err)
	}
	return lines, headings, nil
}

func markdownSection(path string, lines []string, headings []MarkdownHeading, heading string) (ReadMDOutput, error) {
	heading = strings.TrimSpace(heading)
	idx := -1
	for i, candidate := range headings {
		if candidate.Title == heading {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ReadMDOutput{}, fmt.Errorf("heading %q not found in %s", heading, path)
	}
	start := headings[idx].Line
	end := len(lines)
	for _, next := range headings[idx+1:] {
		if next.Level <= headings[idx].Level {
			end = next.Line - 1
			break
		}
	}
	content := strings.Join(lines[start-1:end], "\n")
	if content != "" {
		content += "\n"
	}
	return ReadMDOutput{
		Path:       path,
		Action:     "read_section",
		Heading:    heading,
		StartLine:  start,
		EndLine:    end,
		Content:    content,
		TotalLines: len(lines),
	}, nil
}
