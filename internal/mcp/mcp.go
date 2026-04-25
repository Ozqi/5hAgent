package mcp

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

type ServerConfig struct {
	Name           string            `json:"name" yaml:"name"`
	Command        string            `json:"command" yaml:"command"`
	Args           []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	StartupTimeout time.Duration     `json:"startup_timeout,omitempty" yaml:"startup_timeout,omitempty"`
}

type ToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ReadOnly    bool   `json:"read_only,omitempty"`
}

type Client interface {
	CallTool(ctx context.Context, toolName string, arguments string) (string, error)
}

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

func FullToolName(serverName string, toolName string) string {
	return fmt.Sprintf("mcp.%s.%s", serverName, toolName)
}
