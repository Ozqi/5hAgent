// Package tools 提供 Agent 可调用的本地工具、上下文工具、Skill 工具和 MCP 工具注册能力。
package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Ozqi/walle/internal/mcp"
	"github.com/Ozqi/walle/internal/skill"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// Registry 保存一次 runtime 可见的工具集合。
type Registry struct {
	mu            sync.RWMutex
	tools         []tool.BaseTool
	workspaceRoot string
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry { return &Registry{} }

// SetWorkspaceRoot 设置后续本地工具解析相对路径时使用的工作目录。
func (r *Registry) SetWorkspaceRoot(root string) {
	r.workspaceRoot = root
}

// Init 清空已有工具，再按固定顺序注册基础和 Skill 工具。
// 注册顺序会成为模型可见的工具顺序；context.context 由 Runtime 在 Init 后单独注册。
func (r *Registry) Init(skillMgr *skill.Manager) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools = nil

	baseTools := []struct {
		name string
		fn   func() (tool.BaseTool, error)
	}{
		{name: "base.read_file", fn: func() (tool.BaseTool, error) { return NewReadFileTool(r.workspaceRoot) }},
		{name: "base.read_md", fn: func() (tool.BaseTool, error) { return NewReadMDTool(r.workspaceRoot) }},
		{name: "base.exec_shell", fn: func() (tool.BaseTool, error) { return NewExecShellTool(r.workspaceRoot) }},
		{name: "base.glob", fn: func() (tool.BaseTool, error) { return NewGlobTool(r.workspaceRoot) }},
		{name: "base.edit", fn: func() (tool.BaseTool, error) { return NewEditTool(r.workspaceRoot) }},
		{name: "base.write_file", fn: func() (tool.BaseTool, error) { return NewWriteFileTool(r.workspaceRoot) }},
		{name: "base.grep", fn: func() (tool.BaseTool, error) { return NewGrepTool(r.workspaceRoot) }},
		{name: "base.list_dir", fn: func() (tool.BaseTool, error) { return NewListDirTool(r.workspaceRoot) }},
	}

	for _, t := range baseTools {
		tool, err := t.fn()
		if err != nil {
			return fmt.Errorf("failed to create %s tool: %w", t.name, err)
		}
		r.tools = append(r.tools, tool)
	}

	if skillMgr != nil {
		r.tools = append(r.tools, &SkillTool{mgr: skillMgr})
	}
	return nil
}

// All 返回当前工具切片的副本，避免调用方修改注册表内部切片。
func (r *Registry) All() []tool.BaseTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]tool.BaseTool, len(r.tools))
	copy(tools, r.tools)
	return tools
}

// ToolInfos 收集当前所有工具的模型可见 schema。
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

// RegisterContextTool 注册依赖当前模型和 prompt 目录的上下文工具，用于 LLM 摘要压缩。
// Runtime 需要在收集 ToolInfos 并调用 WithTools 前完成注册。
func (r *Registry) RegisterContextTool(llm model.ToolCallingChatModel, promptDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = append(r.tools, NewContextTool(llm, promptDir))
}

// ReplaceContextTool 在保留其他工具的前提下替换上下文工具，供运行时切换模型后重新绑定。
// 替换后 Runtime 仍需重新收集 ToolInfos 并调用 WithTools。
func (r *Registry) ReplaceContextTool(llm model.ToolCallingChatModel, promptDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	filtered := r.tools[:0]
	for _, t := range r.tools {
		if _, ok := t.(*ContextTool); ok {
			continue
		}
		filtered = append(filtered, t)
	}
	r.tools = append(filtered, NewContextTool(llm, promptDir))
}

// RegisterMCPTools 将远端 MCP schema 和 client 包装为本地 Eino 工具。
func (r *Registry) RegisterMCPTools(serverName string, client mcp.Client, specs []mcp.ToolSpec) error {
	if serverName == "" {
		return fmt.Errorf("mcp server name is required")
	}
	if client == nil {
		return fmt.Errorf("mcp client is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// MCP 名称、schema 和调用结果均来自进程外部；注册表只做命名隔离。
	for _, spec := range specs {
		if spec.Name == "" {
			return fmt.Errorf("mcp tool name is required")
		}
		r.tools = append(r.tools, NewMCPTool(serverName, client, spec))
	}
	return nil
}

// DisplayName 返回工具名的最后一段，用于终端展示。
func DisplayName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx+1 < len(name) {
		return name[idx+1:]
	}
	return name
}
