// skill_tool.go - 技能管理工具
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: skill
// Desc: 查看当前 Agent 启动时加载的 skill（技能模块）。
//
//	Skill 是预定义的 Agent 能力扩展，通过 skill 文件定义。
//
// Input Parameters:
//   - action (string, optional)  : 操作类型：list（默认）/ get
//   - skill  (string, optional)  : get 时使用的技能名称
//
// Error Scenarios (LLM Hints):
//   - MISSING 'skill'             → get 时必须提供技能名称
//   - skill not found             → 技能名称不存在；检查技能列表
//   - unknown action              → action 必须是 list 或 get
//
// Tips:
//   - skill 集合在 Agent 启动时固定，运行期不支持启用或禁用
//
// ============================================================
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/skill"
)

// --- LLM 描述常量 ---
const (
	skillToolName = "skill.skill"
	skillToolDesc = `Inspect skills loaded for this Agent process. Skills are fixed at startup from global and project directories.
- action: optional, one of list or get; defaults to list
- skill: required only for action=get, exact skill name
Example: {"action":"list"} or {"action":"get","skill":"systematic-debugging"}`
	skillToolErrors = `MISSING 'skill': get 时必须提供技能名称
skill not found: 技能名称不存在；检查技能列表
unknown action: action 必须是 list 或 get`
	skillToolTips = `skill 集合在 Agent 启动时固定，运行期不支持启用或禁用`
)

type SkillTool struct{ mgr *skill.Manager }

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
		skill, ok := t.mgr.GetSkill(input.Skill)
		if !ok {
			return "", fmt.Errorf("skill not found: %s", input.Skill)
		}
		return fmt.Sprintf("# Skill: %s\n\nscope: %s\npath: %s\n\n%s", skill.Name, skill.Scope, skill.Path, skill.Content), nil
	}
	return "", fmt.Errorf("unknown action '%s'. Use 'list' or 'get'.", action)
}

var _ tool.InvokableTool = (*SkillTool)(nil)
