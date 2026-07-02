// registry.go - 工具注册表
// 功能：集中注册基础工具、Task 工具、Skill 工具、MCP 工具
// 导出函数：InitRegistry, GetAllTools, GetToolByName, RegisterMCPTools
package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/lzq/5hAgent/internal/mcp"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/toolmeta"
)

// registry holds all registered tools
var registry []tool.BaseTool
var registryMu sync.RWMutex

// mcpServers 注册的 MCP 服务器
var mcpServers map[string]mcp.Client
var mcpServerDescs map[string]string

// InitRegistry 初始化工具注册表（需要在 main 中调用）
func InitRegistry(taskList *task.TaskList, skillMgr *skill.Manager) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry = nil // 清空
	toolmeta.Reset()
	mcpServers = make(map[string]mcp.Client)
	mcpServerDescs = make(map[string]string)

	// 基础文件工具
	baseTools := []struct {
		meta toolmeta.Meta
		fn   func() (tool.BaseTool, error)
	}{
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "read_file", FullName: "base.read_file", OriginalName: "read_file", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewReadFileTool() }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "exec_shell", FullName: "base.exec_shell", OriginalName: "exec_shell"}, fn: func() (tool.BaseTool, error) { return NewExecShellTool() }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "glob", FullName: "base.glob", OriginalName: "glob", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewGlobTool() }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "edit", FullName: "base.edit", OriginalName: "edit"}, fn: func() (tool.BaseTool, error) { return NewEditTool() }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "write_file", FullName: "base.write_file", OriginalName: "write_file"}, fn: func() (tool.BaseTool, error) { return NewWriteFileTool() }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "grep", FullName: "base.grep", OriginalName: "grep", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewGrepTool() }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "list_dir", FullName: "base.list_dir", OriginalName: "list_dir", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewListDirTool() }},
	}

	for _, t := range baseTools {
		tool, err := t.fn()
		if err != nil {
			return fmt.Errorf("failed to create %s tool: %w", t.meta.FullName, err)
		}
		registry = append(registry, tool)
		toolmeta.Register(t.meta)
	}

	// Task 工具（统一入口）
	if taskList != nil {
		registry = append(registry, &TaskTool{taskList: taskList})
		toolmeta.Register(toolmeta.Meta{Category: toolmeta.CategoryTask, Source: "local", DisplayName: "task", FullName: "task.task", OriginalName: "task"})
	}

	// Skill 工具
	if skillMgr != nil {
		registry = append(registry, &SkillTool{mgr: skillMgr})
		toolmeta.Register(toolmeta.Meta{Category: toolmeta.CategorySkill, Source: "local", DisplayName: "skill", FullName: "skill.skill", OriginalName: "skill"})
	}

	return nil
}

func ensureRegistry() {
	registryMu.RLock()
	initialized := len(registry) > 0
	registryMu.RUnlock()
	if initialized {
		return
	}

	_ = InitRegistry(nil, nil)
}

// GetAllTools returns all registered tools
func GetAllTools() []tool.BaseTool {
	ensureRegistry()
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry
}

// GetToolByName returns a tool by its name, or nil if not found
func GetToolByName(name string) tool.BaseTool {
	ensureRegistry()
	registryMu.RLock()
	defer registryMu.RUnlock()

	ctx := context.Background()
	for _, t := range registry {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == name {
			return t
		}
		if meta, ok := toolmeta.Lookup(info.Name); ok {
			if meta.DisplayName == name || meta.OriginalName == name {
				return t
			}
		}
	}
	return nil
}

func RegisterMCPTools(serverName string, client mcp.Client, specs []mcp.ToolSpec) error {
	if serverName == "" {
		return fmt.Errorf("mcp server name is required")
	}
	if client == nil {
		return fmt.Errorf("mcp client is required")
	}

	// 注册服务器到全局 map（供 mcp_list_tools 使用）
	RegisterMCPServer(serverName, client)

	for _, spec := range specs {
		if spec.Name == "" {
			return fmt.Errorf("mcp tool name is required")
		}
		registry = append(registry, NewMCPTool(serverName, client, spec))
		toolmeta.Register(toolmeta.Meta{
			Category:     toolmeta.CategoryMCP,
			Source:       serverName,
			DisplayName:  spec.Name,
			FullName:     mcp.FullToolName(serverName, spec.Name),
			OriginalName: spec.Name,
			ReadOnly:     spec.ReadOnly,
		})
	}
	return nil
}

// RegisterMCPServer 注册 MCP 服务器
func RegisterMCPServer(name string, client mcp.Client) {
	mcpServers[name] = client
}

// GetMCPServers 返回所有注册的 MCP 服务器
func GetMCPServers() map[string]mcp.Client {
	return mcpServers
}

// GetMCPServer 返回指定名称的 MCP 服务器
func GetMCPServer(name string) (mcp.Client, bool) {
	client, ok := mcpServers[name]
	return client, ok
}

// DisplayName returns the display name for a tool
func DisplayName(name string) string {
	return toolmeta.DisplayName(name)
}

// Lookup returns the meta for a tool name
func Lookup(name string) (toolmeta.Meta, bool) {
	return toolmeta.Lookup(name)
}

// Re-export Category constants
const (
	CategoryBase  = toolmeta.CategoryBase
	CategoryTask  = toolmeta.CategoryTask
	CategorySkill = toolmeta.CategorySkill
	CategoryMCP   = toolmeta.CategoryMCP
)
