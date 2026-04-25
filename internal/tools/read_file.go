package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
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
		"base.read_file",
		"Read file content from the specified path. Returns the content and total line count. Supports reading specific line ranges using offset and limit parameters.",
		func(ctx context.Context, input ReadFileInput) (*schema.ToolResult, error) {
			// Validate path
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the file path to read")
			}

			// Set default values
			if input.Offset == 0 {
				input.Offset = 1
			}
			if input.Limit == 0 {
				input.Limit = 100
			}

			// Validate offset
			if input.Offset < 1 {
				return nil, fmt.Errorf("offset must be >= 1, got %d", input.Offset)
			}

			// Open file
			file, err := os.Open(input.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to open file '%s': %w. Make sure the path is correct and the file exists", input.Path, err)
			}
			defer file.Close()

			// Read lines
			scanner := bufio.NewScanner(file)
			var lines []string
			lineNum := 0
			totalLines := 0

			for scanner.Scan() {
				totalLines++
				lineNum++

				// Skip lines before offset
				if lineNum < input.Offset {
					continue
				}

				// Stop if we've read enough lines
				if len(lines) >= input.Limit {
					// Continue counting total lines
					for scanner.Scan() {
						totalLines++
					}
					break
				}

				lines = append(lines, scanner.Text())
			}

			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading file: %w", err)
			}

			// Build output
			output := ReadFileOutput{
				Content:    "",
				TotalLines: totalLines,
			}

			// Format content with line numbers
			for i, line := range lines {
				lineNumber := input.Offset + i
				output.Content += fmt.Sprintf("%d\t%s\n", lineNumber, line)
			}

			// Convert output to JSON string
			outputJSON, err := json.Marshal(output)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal output: %w", err)
			}

			// Return as ToolResult with text part
			return &schema.ToolResult{
				Parts: []schema.ToolOutputPart{
					{
						Type: schema.ToolPartTypeText,
						Text: string(outputJSON),
					},
				},
			}, nil
		},
	)
}
