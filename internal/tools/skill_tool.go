// skill_tool.go - 技能管理工具（LLM 调用入口）
// 功能：封装 SkillManager 为 Eino Tool，供 Agent 调用
// 主要类型：SkillTool
// 导出函数：NewSkillTool
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/skill"
)

// SkillTool LLM 调用 skill 的工具
type SkillTool struct {
	mgr *skill.Manager
}

func NewSkillTool(mgr *skill.Manager) *SkillTool {
	return &SkillTool{mgr: mgr}
}

func (t *SkillTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill.skill",
		Desc: "Enable or disable a skill. Skills provide specialized capabilities for specific tasks.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill": {
				Type:     schema.String,
				Desc:     "Name of the skill to enable",
				Required: true,
			},
			"action": {
				Type:     schema.String,
				Desc:     "Action to perform: 'enable' or 'disable' (default: enable)",
				Required: false,
			},
		}),
	}, nil
}

func (t *SkillTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input struct {
		Skill  string `json:"skill"`
		Action string `json:"action,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	if input.Skill == "" {
		return "", fmt.Errorf("skill name is required")
	}

	// 默认 action 是 enable
	action := input.Action
	if action == "" {
		action = "enable"
	}

	switch action {
	case "enable":
		if err := t.mgr.EnableSkill(input.Skill); err != nil {
			return "", err
		}
		return fmt.Sprintf("Skill '%s' enabled", input.Skill), nil
	case "disable":
		if err := t.mgr.DisableSkill(input.Skill); err != nil {
			return "", err
		}
		return fmt.Sprintf("Skill '%s' disabled", input.Skill), nil
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

var _ tool.InvokableTool = (*SkillTool)(nil)
