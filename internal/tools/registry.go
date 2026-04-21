package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/skill"
)

// registry holds all registered tools
var registry []tool.BaseTool

// InitRegistry 初始化工具注册表（需要在 main 中调用）
func InitRegistry(taskList *agent.TaskList, skillMgr *skill.Manager) error {
	registry = nil // 清空

	// 基础文件工具
	tools := []struct {
		name string
		fn   func() (tool.BaseTool, error)
	}{
		{"read_file", func() (tool.BaseTool, error) { return NewReadFileTool() }},
		{"exec_shell", func() (tool.BaseTool, error) { return NewExecShellTool() }},
		{"glob", func() (tool.BaseTool, error) { return NewGlobTool() }},
		{"edit", func() (tool.BaseTool, error) { return NewEditTool() }},
		{"write_file", func() (tool.BaseTool, error) { return NewWriteFileTool() }},
		{"grep", func() (tool.BaseTool, error) { return NewGrepTool() }},
		{"list_dir", func() (tool.BaseTool, error) { return NewListDirTool() }},
	}

	for _, t := range tools {
		tool, err := t.fn()
		if err != nil {
			return fmt.Errorf("failed to create %s tool: %w", t.name, err)
		}
		registry = append(registry, tool)
	}

	// Task 工具（统一入口）
	if taskList != nil {
		registry = append(registry, NewTaskTool(taskList))
	}

	// Skill 工具
	if skillMgr != nil {
		registry = append(registry, NewSkillTool(skillMgr))
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
