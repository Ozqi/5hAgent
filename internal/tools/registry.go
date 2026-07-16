// registry.go - 工具注册表
// 功能：集中注册基础工具、Task 工具、Skill 工具、MCP 工具
// 导出函数：NewRegistry
package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/mcp"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/toolmeta"
)

// Registry 保存一次 runtime 可见的工具集合。
type Registry struct {
	mu            sync.RWMutex
	tools         []tool.BaseTool
	meta          map[string]toolmeta.Meta
	mcpServers    map[string]mcp.Client
	workspaceRoot string
}

func NewRegistry() *Registry {
	return &Registry{
		meta:       make(map[string]toolmeta.Meta),
		mcpServers: make(map[string]mcp.Client),
	}
}

func (r *Registry) SetWorkspaceRoot(root string) {
	r.workspaceRoot = root
}

func (r *Registry) Init(taskList *task.TaskList, skillMgr *skill.Manager) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools = nil
	r.meta = make(map[string]toolmeta.Meta)
	r.mcpServers = make(map[string]mcp.Client)

	// 基础文件工具
	baseTools := []struct {
		meta toolmeta.Meta
		fn   func() (tool.BaseTool, error)
	}{
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "read_file", FullName: "base.read_file", OriginalName: "read_file", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewReadFileTool(r.workspaceRoot) }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "read_md", FullName: "base.read_md", OriginalName: "read_md", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewReadMDTool(r.workspaceRoot) }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "exec_shell", FullName: "base.exec_shell", OriginalName: "exec_shell"}, fn: func() (tool.BaseTool, error) { return NewExecShellTool(r.workspaceRoot) }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "glob", FullName: "base.glob", OriginalName: "glob", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewGlobTool(r.workspaceRoot) }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "edit", FullName: "base.edit", OriginalName: "edit"}, fn: func() (tool.BaseTool, error) { return NewEditTool(r.workspaceRoot) }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "write_file", FullName: "base.write_file", OriginalName: "write_file"}, fn: func() (tool.BaseTool, error) { return NewWriteFileTool(r.workspaceRoot) }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "grep", FullName: "base.grep", OriginalName: "grep", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewGrepTool(r.workspaceRoot) }},
		{meta: toolmeta.Meta{Category: toolmeta.CategoryBase, Source: "local", DisplayName: "list_dir", FullName: "base.list_dir", OriginalName: "list_dir", ReadOnly: true}, fn: func() (tool.BaseTool, error) { return NewListDirTool(r.workspaceRoot) }},
	}

	for _, t := range baseTools {
		tool, err := t.fn()
		if err != nil {
			return fmt.Errorf("failed to create %s tool: %w", t.meta.FullName, err)
		}
		r.tools = append(r.tools, tool)
		r.registerMeta(t.meta)
	}

	// Task 工具（统一入口）
	if taskList != nil {
		r.tools = append(r.tools, &TaskTool{taskList: taskList})
		r.registerMeta(toolmeta.Meta{Category: toolmeta.CategoryTask, Source: "local", DisplayName: "task", FullName: "task.task", OriginalName: "task"})
	}

	// Skill 工具
	if skillMgr != nil {
		r.tools = append(r.tools, &SkillTool{mgr: skillMgr})
		r.registerMeta(toolmeta.Meta{Category: toolmeta.CategorySkill, Source: "local", DisplayName: "skill", FullName: "skill.skill", OriginalName: "skill", ReadOnly: true})
	}

	return nil
}

func (r *Registry) All() []tool.BaseTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]tool.BaseTool, len(r.tools))
	copy(tools, r.tools)
	return tools
}

func (r *Registry) ToolInfos(ctx context.Context) ([]*schema.ToolInfo, error) {
	r.mu.RLock()
	tools := make([]tool.BaseTool, len(r.tools))
	copy(tools, r.tools)
	r.mu.RUnlock()

	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (r *Registry) Get(name string) tool.BaseTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ctx := context.Background()
	for _, t := range r.tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == name {
			return t
		}
		if meta, ok := r.lookupMeta(info.Name); ok {
			if meta.DisplayName == name || meta.OriginalName == name {
				return t
			}
		}
	}
	return nil
}

func (r *Registry) RegisterContextTool(llm model.ToolCallingChatModel, promptDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = append(r.tools, NewContextTool(llm, promptDir))
	r.registerMeta(toolmeta.Meta{Category: toolmeta.CategoryContext, Source: "local", DisplayName: "context", FullName: "context.context", OriginalName: "context"})
}

func (r *Registry) ReplaceContextTool(llm model.ToolCallingChatModel, promptDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	filtered := r.tools[:0]
	for _, t := range r.tools {
		info, err := t.Info(context.Background())
		if err == nil && info.Name == "context.context" {
			continue
		}
		filtered = append(filtered, t)
	}
	r.tools = append(filtered, NewContextTool(llm, promptDir))
	r.registerMeta(toolmeta.Meta{Category: toolmeta.CategoryContext, Source: "local", DisplayName: "context", FullName: "context.context", OriginalName: "context"})
}

func (r *Registry) RegisterMCPTools(serverName string, client mcp.Client, specs []mcp.ToolSpec) error {
	if serverName == "" {
		return fmt.Errorf("mcp server name is required")
	}
	if client == nil {
		return fmt.Errorf("mcp client is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mcpServers == nil {
		r.mcpServers = make(map[string]mcp.Client)
	}
	r.mcpServers[serverName] = client

	for _, spec := range specs {
		if spec.Name == "" {
			return fmt.Errorf("mcp tool name is required")
		}
		r.tools = append(r.tools, NewMCPTool(serverName, client, spec))
		r.registerMeta(toolmeta.Meta{
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

// DisplayName returns the display name for a tool
func DisplayName(name string) string {
	return toolmeta.DisplayNameFallback(name)
}

func (r *Registry) registerMeta(meta toolmeta.Meta) {
	if meta.FullName == "" {
		return
	}
	if r.meta == nil {
		r.meta = make(map[string]toolmeta.Meta)
	}
	r.meta[meta.FullName] = meta
}

func (r *Registry) lookupMeta(name string) (toolmeta.Meta, bool) {
	meta, ok := r.meta[name]
	return meta, ok
}
