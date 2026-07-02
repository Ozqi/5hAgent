// skill_tool.go - 技能管理工具
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/skill"
)

type SkillTool struct{ mgr *skill.Manager }

func NewSkillTool(m *skill.Manager) *SkillTool { return &SkillTool{mgr: m} }

func (t *SkillTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill.skill",
		Desc: "Enable or disable a skill.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill":  {Type: schema.String, Desc: "Skill name", Required: true},
			"action": {Type: schema.String, Desc: "enable/disable (default: enable)"},
		}),
	}, nil
}

func (t *SkillTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input struct {
		Skill  string `json:"skill"`
		Action string `json:"action,omitempty"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", err
	}
	if input.Skill == "" {
		return "", fmt.Errorf("skill name required")
	}
	action := input.Action
	if action == "" {
		action = "enable"
	}
	switch action {
	case "enable":
		return fmt.Sprintf("Skill '%s' enabled", input.Skill), t.mgr.EnableSkill(input.Skill)
	case "disable":
		return fmt.Sprintf("Skill '%s' disabled", input.Skill), t.mgr.DisableSkill(input.Skill)
	}
	return "", fmt.Errorf("unknown action: %s", action)
}

var _ tool.InvokableTool = (*SkillTool)(nil)
