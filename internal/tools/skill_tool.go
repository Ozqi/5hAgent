package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ozqi/walle/internal/skill"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// --- LLM 描述常量 ---
const (
	skillToolName = "skill.skill"
	skillToolDesc = `Inspect the current skill snapshot for this Agent process. Skills load from global and project directories and may be explicitly refreshed with /skill reload.
- action: optional, one of list or get; defaults to list
- skill: required only for action=get, exact skill name
Example: {"action":"list"} or {"action":"get","skill":"systematic-debugging"}`
)

// SkillTool 将 skill 快照查询适配为 Eino 工具。
type SkillTool struct{ mgr *skill.Manager }

// Info 返回 skill.skill 的模型可见名称、说明和参数约束。
func (t *SkillTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: skillToolName,
		Desc: skillToolDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill":  {Type: schema.String, Desc: "Exact skill name; required only when action=get"},
			"action": {Type: schema.String, Desc: "One of: list, get. Defaults to list.", Enum: []string{"list", "get"}},
		}),
	}, nil
}

// InvokableRun 校验查询动作，并将已加载 skill 的元数据或正文返回给模型。
func (t *SkillTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input struct {
		Skill  string `json:"skill"`
		Action string `json:"action,omitempty"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("invalid skill arguments: %w. Expected JSON with action=list or action=get.", err)
	}
	action := input.Action
	if action == "" {
		action = "list"
	}
	switch action {
	case "list":
		skills := t.mgr.ListSkills()
		if len(skills) == 0 {
			return "No skills loaded", nil
		}
		result := "Loaded skills:\n"
		for _, skill := range skills {
			result += fmt.Sprintf("- %s [%s]: %s\n", skill.Name, skill.Scope, skill.Description)
		}
		return result, nil
	case "get":
		if input.Skill == "" {
			return "", fmt.Errorf("MISSING REQUIRED PARAMETER: 'skill' is required for action=get.")
		}
		// skill 正文会进入后续模型上下文；这里只允许按当前快照中的精确名称读取。
		skill, ok := t.mgr.GetSkill(input.Skill)
		if !ok {
			return "", fmt.Errorf("skill not found: %s", input.Skill)
		}
		return fmt.Sprintf("# Skill: %s\n\nscope: %s\npath: %s\n\n%s", skill.Name, skill.Scope, skill.Path, skill.Content), nil
	}
	return "", fmt.Errorf("unknown action '%s'. Use 'list' or 'get'.", action)
}

var _ tool.InvokableTool = (*SkillTool)(nil)
