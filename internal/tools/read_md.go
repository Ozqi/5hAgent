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
- action: required, one of list_headings, read_section, replace_section, or delete_section.
- path: required Markdown file path.
- heading: required for section actions. Match heading text exactly, without leading #.
- content: required for replace_section. Include the replacement heading line itself.
Use list_headings before a section action when unsure about the exact heading.`
)

// ReadMDInput 描述结构化 Markdown 操作及其参数。
type ReadMDInput struct {
	Action  string `json:"action" jsonschema:"required,description=Action to run: list_headings, read_section, replace_section, or delete_section."`
	Path    string `json:"path" jsonschema:"required,description=Required Markdown file path."`
	Heading string `json:"heading,omitempty" jsonschema:"description=Required for section actions. Exact heading text without leading #."`
	Content string `json:"content,omitempty" jsonschema:"description=Required for replace_section. Full replacement section including its heading line."`
}

// MarkdownHeading 记录标题层级、文本和一基行号。
type MarkdownHeading struct {
	Level int    `json:"level"`
	Title string `json:"title"`
	Line  int    `json:"line"`
}

// ReadMDOutput 返回标题列表、section 范围或写入结果。
type ReadMDOutput struct {
	Success    bool              `json:"success,omitempty"`
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

// NewReadMDTool 创建支持 Markdown section 读写的 Eino 工具。
func NewReadMDTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		readMDToolName,
		readMDToolDesc,
		func(ctx context.Context, input ReadMDInput) (*schema.ToolResult, error) {
			// 模型输入在这里进入文件系统信任边界；路径解析不提供 workspace confinement。
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
			case "replace_section", "delete_section":
				if strings.TrimSpace(input.Heading) == "" {
					return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'heading' is required for %s", input.Action)
				}
				if input.Action == "replace_section" && strings.TrimSpace(input.Content) == "" {
					return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'content' is required for replace_section")
				}
				return writeMarkdownSection(input.Path, lines, headings, input)
			default:
				return nil, fmt.Errorf("unknown action %q: expected list_headings, read_section, replace_section, or delete_section", input.Action)
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
	start, end, err := markdownSectionRange(path, lines, headings, heading)
	if err != nil {
		return ReadMDOutput{}, err
	}
	content := strings.Join(lines[start-1:end], "\n")
	if content != "" {
		content += "\n"
	}
	return ReadMDOutput{
		Path:       path,
		Action:     "read_section",
		Heading:    strings.TrimSpace(heading),
		StartLine:  start,
		EndLine:    end,
		Content:    content,
		TotalLines: len(lines),
	}, nil
}

func markdownSectionRange(path string, lines []string, headings []MarkdownHeading, heading string) (int, int, error) {
	heading = strings.TrimSpace(heading)
	idx := -1
	for i, candidate := range headings {
		if candidate.Title == heading {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, 0, fmt.Errorf("heading %q not found in %s", heading, path)
	}
	start := headings[idx].Line
	end := len(lines)
	for _, next := range headings[idx+1:] {
		if next.Level <= headings[idx].Level {
			end = next.Line - 1
			break
		}
	}
	return start, end, nil
}

func writeMarkdownSection(path string, lines []string, headings []MarkdownHeading, input ReadMDInput) (*schema.ToolResult, error) {
	// 先在内存中重建完整文件，再沿用原权限覆盖；写入不是原子替换。
	start, end, err := markdownSectionRange(path, lines, headings, input.Heading)
	if err != nil {
		return nil, err
	}
	updated := append([]string{}, lines[:start-1]...)
	if input.Content != "" {
		// Split 保留末尾空元素，让调用方给出的 section 尾部换行继续分隔下一个标题。
		updated = append(updated, strings.Split(input.Content, "\n")...)
	}
	updated = append(updated, lines[end:]...)
	content := strings.Join(updated, "\n")
	if len(updated) > 0 {
		content += "\n"
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat Markdown file %q: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("%s heading %q in %s: %w", input.Action, input.Heading, path, err)
	}
	return JSONResult(ReadMDOutput{
		Success:    true,
		Path:       path,
		Action:     input.Action,
		Heading:    strings.TrimSpace(input.Heading),
		StartLine:  start,
		EndLine:    end,
		TotalLines: len(updated),
	})
}
