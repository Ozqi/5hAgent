package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// WriteFileInput defines the input parameters for write_file tool
type WriteFileInput struct {
	Path    string `json:"path" jsonschema:"required,description=Absolute path to the file to write"`
	Content string `json:"content" jsonschema:"required,description=Content to write to the file"`
}

// WriteFileOutput defines the output structure for write_file tool
type WriteFileOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Bytes   int    `json:"bytes"`
}

// NewWriteFileTool creates a new write_file tool using Eino's InferEnhancedTool
func NewWriteFileTool() (tool.EnhancedInvokableTool, error) {
	return utils.InferEnhancedTool(
		"base.write_file",
		"Write content to a file at the specified path. Creates the file if it doesn't exist, or overwrites it if it does. Automatically creates parent directories if needed.",
		func(ctx context.Context, input WriteFileInput) (*schema.ToolResult, error) {
			// Validate input
			if input.Path == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'path' is required. You must provide the file path to write")
			}
			if input.Content == "" {
				return nil, fmt.Errorf("MISSING REQUIRED PARAMETER: 'content' is required. You must provide the content to write")
			}

			// Create parent directories if they don't exist
			dir := filepath.Dir(input.Path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create parent directories for '%s': %w", input.Path, err)
			}

			// Write file
			err := os.WriteFile(input.Path, []byte(input.Content), 0644)
			if err != nil {
				return nil, fmt.Errorf("failed to write file '%s': %w", input.Path, err)
			}

			// Build output
			output := WriteFileOutput{
				Success: true,
				Message: fmt.Sprintf("Successfully wrote to %s", input.Path),
				Bytes:   len(input.Content),
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
