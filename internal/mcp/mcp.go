// Package mcp 实现 MCP server 配置、stdio JSON-RPC 客户端和工具发现接口。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ServerConfig 描述一个通过命令和 stdio 启动的 MCP server。
type ServerConfig struct {
	Name           string            `json:"name" yaml:"name"`
	Command        string            `json:"command" yaml:"command"`
	Args           []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	StartupTimeout time.Duration     `json:"startup_timeout,omitempty" yaml:"startup_timeout,omitempty"`
}

// ToolSpec 描述 MCP server 暴露的工具及其输入 JSON Schema。
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	ReadOnly    bool            `json:"read_only,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// Client 是工具注册层调用 MCP 工具所需的最小接口。
type Client interface {
	CallTool(ctx context.Context, toolName string, arguments string) (string, error)
}

// Validate 校验 server 名称和启动命令是否满足注册要求。
func (c ServerConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("mcp server name is required")
	}
	if !validNamePattern.MatchString(c.Name) {
		return fmt.Errorf("invalid mcp server name: %s", c.Name)
	}
	if c.Command == "" {
		return fmt.Errorf("mcp server command is required")
	}
	return nil
}

// FullToolName 将 server 和工具名拼为 LLM 可见的 mcp.<server>.<tool> 名称。
func FullToolName(serverName string, toolName string) string {
	return fmt.Sprintf("mcp.%s.%s", serverName, toolName)
}
