package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// --- LLM 描述常量（供 InferEnhancedTool 使用）---
const (
	execShellToolName = "base.exec_shell"
	execShellToolDesc = `Execute a shell command and return stdout, stderr, and return code. Always send JSON object arguments.
- command: required shell command string.
Prefer dedicated tools for files/search: read_file, edit, write_file, grep, glob, list_dir.
Quote paths with spaces. Avoid destructive commands unless explicitly requested.`
)

// ExecShellInput 描述 exec_shell 工具的输入参数
type ExecShellInput struct {
	Command string `json:"command" jsonschema:"required,description=Required shell command to execute. Quote paths with spaces. Prefer dedicated file/search tools when possible."`
}

// ExecShellOutput 描述 exec_shell 工具的输出结构
type ExecShellOutput struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ReturnCode int    `json:"returncode"`
}

// NewExecShellTool 使用 Eino InferEnhancedTool 创建 exec_shell 工具
func NewExecShellTool(workspaceRoot ...string) (tool.EnhancedInvokableTool, error) {
	root := ""
	if len(workspaceRoot) > 0 {
		root = workspaceRoot[0]
	}
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			root = "."
		}
	}

	return utils.InferEnhancedTool(
		execShellToolName,
		fmt.Sprintf("%s (workspace root: %s)", execShellToolDesc, root),
		func(ctx context.Context, input ExecShellInput) (*schema.ToolResult, error) {
			// command 是模型提供的完整 shell 程序，属于最高风险信任边界；此处仅拒绝空命令。
			if input.Command == "" {
				return nil, fmt.Errorf("command cannot be empty. Provide a valid shell command to execute.")
			}

			// CommandContext 负责取消子进程；sh -c 会执行重定向、管道、展开及命令中的全部副作用。
			cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)
			cmd.Dir = root

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			var returnCode int

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					returnCode = exitErr.ExitCode()
				} else {
					return nil, fmt.Errorf("failed to execute command: %w. Check that the command exists and is in your PATH.", err)
				}
			} else {
				returnCode = 0
			}

			output := ExecShellOutput{Stdout: stdout.String(), Stderr: stderr.String(), ReturnCode: returnCode}
			return JSONResult(output)
		},
	)
}
