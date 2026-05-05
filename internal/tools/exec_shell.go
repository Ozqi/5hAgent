// exec_shell.go - Shell 执行工具
// 功能：执行 shell 命令，返回 stdout/stderr/returncode
// 主要类型：ExecShellInput, ExecShellOutput
// 导出函数：NewExecShellTool
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: exec_shell
// Desc: 执行 shell 命令，返回 stdout、stderr 和返回码。
//
//	可执行系统命令、脚本、CLI 工具。是唯一有副作用的工具。
//
// Input Parameters:
//   - command (string, required)  : 要执行的 shell 命令
//
// Error Scenarios (LLM Hints):
//   - empty command              → command 不能为空
//   - command not found          → 命令不存在或不在 PATH 中；检查命令是否正确安装
//   - context cancelled           → 命令执行超时或被取消；简化命令或分步执行
//   - non-zero exit code         → 命令执行失败；查看 stderr 定位错误原因
//   - permission denied          → 无执行权限；检查命令文件权限
//
// Tips:
//   - 优先使用专门的工具（read_file/edit/write_file/grep/glob）而非 exec_shell
//   - 命令失败时，查看 stderr 而非只依赖 stdout
//   - 破坏性操作（rm -rf、dd 等）请先确认路径和参数
//
// ============================================================
package tools

import (
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
	execShellToolErrors = `empty command: command 不能为空
command not found: 命令不存在或不在 PATH 中；检查命令是否正确安装
context cancelled: 命令执行超时或被取消；简化命令或分步执行
non-zero exit code: 命令执行失败；查看 stderr 定位错误原因
permission denied: 无执行权限；检查命令文件权限`
	execShellToolTips = `优先使用专门的工具（read_file/edit/write_file/grep/glob）而非 exec_shell
命令失败时，查看 stderr 而非只依赖 stdout
破坏性操作（rm -rf、dd 等）请先确认路径和参数`
)

// ExecShellInput defines the input parameters for exec_shell tool
type ExecShellInput struct {
	Command string `json:"command" jsonschema:"required,description=Required shell command to execute. Quote paths with spaces. Prefer dedicated file/search tools when possible."`
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
		execShellToolName,
		fmt.Sprintf("%s (workspace root: %s)", execShellToolDesc, workspaceRoot),
		func(ctx context.Context, input ExecShellInput) (*schema.ToolResult, error) {
			if input.Command == "" {
				return nil, fmt.Errorf("command cannot be empty. Provide a valid shell command to execute.")
			}

			cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)

			stdout, err := cmd.Output()
			var stderr []byte
			var returnCode int

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					stderr = exitErr.Stderr
					returnCode = exitErr.ExitCode()
				} else {
					return nil, fmt.Errorf("failed to execute command: %w. Check that the command exists and is in your PATH.", err)
				}
			} else {
				returnCode = 0
			}

			output := ExecShellOutput{Stdout: string(stdout), Stderr: string(stderr), ReturnCode: returnCode}
			return JSONResult(output)
		},
	)
}
