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
	readFileToolDesc = `按行读取文件，返回带行号的内容和总行数，支持 offset/limit 分段读取。
- path: 文件绝对路径（必填）
- offset: 从第几行开始读，默认1
- limit: 最多读多少行，默认100`
	readFileToolErrors = `MISSING 'path': 必须提供文件绝对路径，不能为空
offset < 1: offset 必须 >= 1
file not found / permission denied: 路径可能错误，确认文件存在；或换用 list_dir/glob 确认路径
read error: 文件可能被占用或损坏；检查 limit 是否过大`
	readFileToolTips = `先用 glob/list_dir 确认文件路径，再调用 read_file
大文件请用 offset+limit 分段读取`
)

// ReadFileInput defines the input parameters for read_file tool
type ReadFileInput struct {
	Path   string `json:"path" jsonschema:"required,description=Absolute path to the file to read"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=Line number to start reading from (default: 1)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Number of lines to read (default: 100)"`
}

// ReadFileOutput defines the output structure for read_file tool
type ReadFileOutput struct {
	Content    string `json:"content"`
	TotalLines int    `json:"total_lines"`
}

// NewReadFileTool creates a new read_file tool using Eino's InferEnhancedTool
func NewReadFileTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		readFileToolName,
		readFileToolDesc,
		func(ctx context.Context, input ReadFileInput) (*schema.ToolResult, error) {
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the file path to read")
			}

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
