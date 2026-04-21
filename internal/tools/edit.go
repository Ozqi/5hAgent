package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// EditInput defines the input parameters for edit tool
type EditInput struct {
	Path      string `json:"path" jsonschema:"required,description=Absolute path to the file to edit"`
	OldString string `json:"old_string" jsonschema:"required,description=Exact string to replace (must match exactly)"`
	NewString string `json:"new_string" jsonschema:"required,description=New string to replace with"`
}

// EditOutput defines the output structure for edit tool
type EditOutput struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	Replacements int    `json:"replacements"`
}

// NewEditTool creates a new edit tool for precise file editing
func NewEditTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"edit",
		"Edit a file by replacing exact string matches. REQUIRED: path (absolute file path), old_string (exact match), new_string (replacement). Returns the number of replacements made.",
		func(ctx context.Context, input EditInput) (*schema.ToolResult, error) {
			// Validate required parameters
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the absolute file path (e.g., '/home/user/project/file.py')")
			}
			if input.OldString == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'old_string' is required. You must provide the exact string to replace")
			}
			if input.NewString == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'new_string' is required. You must provide the replacement string")
			}

			// Read file content
			content, err := os.ReadFile(input.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to read file '%s': %w. Make sure the path is correct and the file exists. Use absolute paths like '/home/user/project/file.py'", input.Path, err)
			}

			originalContent := string(content)

			// Check if old_string exists
			if !strings.Contains(originalContent, input.OldString) {
				output := EditOutput{
					Success:      false,
					Message:      fmt.Sprintf("old_string not found in file. The exact string you provided does not exist in '%s'. Make sure to match whitespace, indentation, and newlines exactly. Consider reading the file again to verify the exact content.", input.Path),
					Replacements: 0,
				}
				outputJSON, _ := json.Marshal(output)
				return &schema.ToolResult{
					Parts: []schema.ToolOutputPart{
						{
							Type: schema.ToolPartTypeText,
							Text: string(outputJSON),
						},
					},
				}, nil
			}

			// Count occurrences
			count := strings.Count(originalContent, input.OldString)

			// Replace all occurrences
			newContent := strings.ReplaceAll(originalContent, input.OldString, input.NewString)

			// Write back to file
			err = os.WriteFile(input.Path, []byte(newContent), 0644)
			if err != nil {
				return nil, fmt.Errorf("failed to write file: %w", err)
			}

			// Build output
			output := EditOutput{
				Success:      true,
				Message:      fmt.Sprintf("Replaced %d occurrence(s)", count),
				Replacements: count,
			}

			outputJSON, err := json.Marshal(output)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal output: %w", err)
			}

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
