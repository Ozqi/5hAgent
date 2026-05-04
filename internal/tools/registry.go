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
)

// registry holds all registered tools
var registry []tool.BaseTool
var registryMu sync.RWMutex

// InitRegistry 初始化工具注册表（需要在 main 中调用）
func InitRegistry(taskList *task.TaskList, skillMgr *skill.Manager) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry = nil // 清空
	Reset()
	mcpServers = make(map[string]mcp.Client)
	mcpServerDescs = make(map[string]string)

	// 基础文件工具
	baseTools := []struct {
		meta Meta
		fn   func() (tool.BaseTool, error)
	}{
		{meta: Meta{Category: CategoryBase, Source: "local", DisplayName: "read_file", FullName: "base.read_file", OriginalName: "read_file", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewReadFileTool() }},
		{meta: Meta{Category: CategoryBase, Source: "local", DisplayName: "exec_shell", FullName: "base.exec_shell", OriginalName: "exec_shell"}, fn: func() (tool.BaseTool, error) { return NewExecShellTool() }},
		{meta: Meta{Category: CategoryBase, Source: "local", DisplayName: "glob", FullName: "base.glob", OriginalName: "glob", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewGlobTool() }},
		{meta: Meta{Category: CategoryBase, Source: "local", DisplayName: "edit", FullName: "base.edit", OriginalName: "edit"}, fn: func() (tool.BaseTool, error) { return NewEditTool() }},
		{meta: Meta{Category: CategoryBase, Source: "local", DisplayName: "write_file", FullName: "base.write_file", OriginalName: "write_file"}, fn: func() (tool.BaseTool, error) { return NewWriteFileTool() }},
		{meta: Meta{Category: CategoryBase, Source: "local", DisplayName: "grep", FullName: "base.grep", OriginalName: "grep", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewGrepTool() }},
		{meta: Meta{Category: CategoryBase, Source: "local", DisplayName: "list_dir", FullName: "base.list_dir", OriginalName: "list_dir", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewListDirTool() }},
		{meta: Meta{Category: CategoryMCP, Source: "local", DisplayName: "mcp_list_tools", FullName: "mcp.list_tools", OriginalName: "mcp_list_tools"}, fn: func() (tool.BaseTool, error) { return NewMCPListToolsTool() }},
	}

	for _, t := range baseTools {
		tool, err := t.fn()
		if err != nil {
			return fmt.Errorf("failed to create %s tool: %w", t.meta.FullName, err)
		}
		registry = append(registry, tool)
		Register(t.meta)
	}

	// Task 工具（统一入口）
	if taskList != nil {
		registry = append(registry, &TaskTool{taskList: taskList})
		Register(Meta{Category: CategoryTask, Source: "local", DisplayName: "task", FullName: "task.task", OriginalName: "task"})
	}

	// Skill 工具
	if skillMgr != nil {
		registry = append(registry, &SkillTool{mgr: skillMgr})
		Register(Meta{Category: CategorySkill, Source: "local", DisplayName: "skill", FullName: "skill.skill", OriginalName: "skill"})
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
		if meta, ok := Lookup(info.Name); ok {
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
		Register(Meta{
			Category:     CategoryMCP,
			Source:       serverName,
			DisplayName:  spec.Name,
			FullName:     mcp.FullToolName(serverName, spec.Name),
			OriginalName: spec.Name,
			ReadOnly:     spec.ReadOnly,
		})
	}
	return nil
}
