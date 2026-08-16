// read_file.go - 文件读取工具
// 功能：按行读取文件，支持 offset/limit 范围指定
// 主要类型：ReadFileInput, ReadFileOutput
// 导出函数：NewReadFileTool
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: read_file
// Desc: 按行读取文件，支持 offset/limit 指定行范围，返回带行号的内容和总行数。
//
//	read_only 工具，不会产生副作用。
//
// Input Parameters:
//   - path     (string, required)  : 文件绝对路径
//   - offset   (int,    optional)  : 从第几行开始读，默认 1
//   - limit    (int,    optional)  : 最多读多少行，默认 100
//
// Error Scenarios (LLM Hints):
//   - MISSING 'path'              → 必须提供文件绝对路径，不能为空
//   - offset < 1                  → offset 必须 >= 1
//   - file not found / no permission
//     → 路径可能错误，确认文件存在；或换用 list_dir/glob 确认路径
//   - read error                  → 文件可能被占用或损坏；检查 limit 是否过大
//
// Tips:
//   - 先用 glob/list_dir 确认文件路径，再调用 read_file
//   - 大文件请用 offset+limit 分段读取
//
// ============================================================
package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// --- LLM 描述常量（供 InferEnhancedTool 使用）---
const (
	readFileToolName = "base.read_file"
	readFileToolDesc = `Read a file by line range and return numbered lines plus total line count. Always send JSON object arguments.
- path: required absolute file path. Use glob/list_dir first if unsure.
- offset: optional 1-based start line. Default 1.
- limit: optional max lines. Default 100. Use smaller chunks for large files.
- Default output is only a prefix chunk, not the whole file. For functions, grep the symbol first and read a small offset range.
- Do not use a partial read to count whole-file occurrences. For counts, use base.grep or base.exec_shell with an exact command.
Example: {"path":"/home/user/project/main.go","offset":1,"limit":120}`
	readFileToolErrors = `MISSING 'path': 必须提供文件绝对路径，不能为空
offset < 1: offset 必须 >= 1
file not found / permission denied: 路径可能错误，确认文件存在；或换用 list_dir/glob 确认路径
read error: 文件可能被占用或损坏；检查 limit 是否过大`
	readFileToolTips = `先用 glob/list_dir 确认文件路径，再调用 read_file
大文件请用 offset+limit 分段读取`
)

// ReadFileInput defines the input parameters for read_file tool
type ReadFileInput struct {
	Path   string `json:"path" jsonschema:"required,description=Required absolute path to the file to read. Use glob/list_dir first if unsure."`
	Offset int    `json:"offset,omitempty" jsonschema:"description=Optional 1-based line number to start reading from. Default: 1."`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Optional number of lines to read. Default: 100. Use 100-200 for large files."`
}

// ReadFileOutput defines the output structure for read_file tool
type ReadFileOutput struct {
	Content    string `json:"content"`
	TotalLines int    `json:"total_lines"`
}

// NewReadFileTool creates a new read_file tool using Eino's InferEnhancedTool
func NewReadFileTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	return utils.InferEnhancedTool(
		readFileToolName,
		readFileToolDesc,
		func(ctx context.Context, input ReadFileInput) (*schema.ToolResult, error) {
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the file path to read")
			}
			input.Path = resolvePath(root, input.Path)

			if input.Offset == 0 {
				input.Offset = 1
			}
			if input.Limit == 0 {
				input.Limit = 100
			}

			if input.Offset < 1 {
				return nil, fmt.Errorf("offset must be >= 1, got %d. Use offset >= 1 to read from a specific line", input.Offset)
			}

			file, err := os.Open(input.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to open file '%s': %w. Make sure the path is correct and the file exists. Use list_dir or glob to verify the path first.", input.Path, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			var lines []string
			lineNum := 0
			totalLines := 0

			for scanner.Scan() {
				totalLines++
				lineNum++
				if lineNum < input.Offset {
					continue
				}
				if len(lines) >= input.Limit {
					for scanner.Scan() {
						totalLines++
					}
					break
				}
				lines = append(lines, scanner.Text())
			}

			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading file: %w. The file may be corrupted or too large. Try reading with a smaller limit.", err)
			}

			output := ReadFileOutput{Content: "", TotalLines: totalLines}
			for i, line := range lines {
				output.Content += fmt.Sprintf("%d\t%s\n", input.Offset+i, line)
			}
			return JSONResult(output)
		},
	)
}
