// exec_shell.go - Shell 执行工具
// 功能：执行 shell 命令，返回 stdout/stderr/returncode
// 主要类型：ExecShellInput, ExecShellOutput
// 导出函数：NewExecShellTool
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// ExecShellInput defines the input parameters for exec_shell tool
type ExecShellInput struct {
	Command string `json:"command" jsonschema:"required,description=Shell command to execute"`
}

// ExecShellOutput defines the output structure for exec_shell tool
type ExecShellOutput struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ReturnCode int    `json:"returncode"`
}

// NewExecShellTool creates a new exec_shell tool using Eino's InferEnhancedTool
func NewExecShellTool() (tool.EnhancedInvokableTool, error) {
	workspaceRoot, err := os.Getwd()
	if err != nil {
		workspaceRoot = "."
	}

	return utils.InferEnhancedTool(
		"base.exec_shell",
		fmt.Sprintf("Execute a shell command and return stdout, stderr, and return code. Use this tool to run system commands, scripts, or CLI tools. The current workspace root is %s. Prefer paths under this workspace instead of guessing unrelated directories.", workspaceRoot),
		func(ctx context.Context, input ExecShellInput) (*schema.ToolResult, error) {
			// Validate input
			if input.Command == "" {
				return nil, fmt.Errorf("command cannot be empty")
			}

			// Execute command using sh -c to support shell features
			cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)

			// Capture stdout and stderr
			stdout, err := cmd.Output()
			var stderr []byte
			var returnCode int

			if err != nil {
				// Check if it's an ExitError (command ran but returned non-zero)
				if exitErr, ok := err.(*exec.ExitError); ok {
					stderr = exitErr.Stderr
					returnCode = exitErr.ExitCode()
				} else {
					// Other errors (e.g., command not found, context cancelled)
					return nil, fmt.Errorf("failed to execute command: %w", err)
				}
			} else {
				returnCode = 0
			}

			// Build output
			output := ExecShellOutput{
				Stdout:     string(stdout),
				Stderr:     string(stderr),
				ReturnCode: returnCode,
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
