package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/toolmeta"
)

// registry holds all registered tools
var registry []tool.BaseTool

// InitRegistry 初始化工具注册表（需要在 main 中调用）
func InitRegistry(taskList *agent.TaskList, skillMgr *skill.Manager) error {
	registry = nil // 清空
	toolmeta.Reset()

	// 基础文件工具
	tools := []struct {
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

	for _, t := range tools {
		tool, err := t.fn()
		if err != nil {
			return fmt.Errorf("failed to create %s tool: %w", t.meta.FullName, err)
		}
		registry = append(registry, tool)
		toolmeta.Register(t.meta)
	}

	// Task 工具（统一入口）
	if taskList != nil {
		registry = append(registry, NewTaskTool(taskList))
		toolmeta.Register(toolmeta.Meta{Category: toolmeta.CategoryTask, Source: "local", DisplayName: "task", FullName: "task.task", OriginalName: "task"})
	}

	// Skill 工具
	if skillMgr != nil {
		registry = append(registry, NewSkillTool(skillMgr))
		toolmeta.Register(toolmeta.Meta{Category: toolmeta.CategorySkill, Source: "local", DisplayName: "skill", FullName: "skill.skill", OriginalName: "skill"})
	}

	return nil
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
