package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
)

// registry holds all registered tools
var registry []tool.BaseTool

// init initializes the tool registry with all available tools
func init() {
	// Register read_file tool
	readFileTool, err := NewReadFileTool()
	if err != nil {
		panic(fmt.Sprintf("failed to create read_file tool: %v", err))
	}
	registry = append(registry, readFileTool)

	// Register exec_shell tool
	execShellTool, err := NewExecShellTool()
	if err != nil {
		panic(fmt.Sprintf("failed to create exec_shell tool: %v", err))
	}
	registry = append(registry, execShellTool)

	// Register glob tool
	globTool, err := NewGlobTool()
	if err != nil {
		panic(fmt.Sprintf("failed to create glob tool: %v", err))
	}
	registry = append(registry, globTool)

	// Register edit tool
	editTool, err := NewEditTool()
	if err != nil {
		panic(fmt.Sprintf("failed to create edit tool: %v", err))
	}
	registry = append(registry, editTool)
}

// GetAllTools returns all registered tools
func GetAllTools() []tool.BaseTool {
	return registry
}

// GetToolByName returns a tool by its name, or nil if not found
func GetToolByName(name string) tool.BaseTool {
	ctx := context.Background()
	for _, t := range registry {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == name {
			return t
		}
	}
	return nil
}
