// skill_tool.go - 技能管理工具
//
// ============================================================
// 工具描述（供人类审阅）
// ============================================================
// Tool: skill
// Desc: 启用或禁用已注册的 skill（技能模块）。
//
//	Skill 是预定义的 Agent 能力扩展，通过 skill 文件定义。
//
// Input Parameters:
//   - skill  (string, required)  : 技能名称
//   - action (string, optional)  : 操作类型：enable（默认）/ disable
//
// Error Scenarios (LLM Hints):
//   - MISSING 'skill'             → 必须提供技能名称
//   - skill not found             → 技能名称不存在；检查技能列表
//   - unknown action              → action 必须是 enable 或 disable
//
// Tips:
//   - skill 在被禁用后不会影响已加载的上下文
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
	skillToolDesc = `启用或禁用已注册的 skill（技能模块）。
- skill: 技能名称（必填）
- action: 操作类型，enable（默认）或 disable（可选）`
	skillToolErrors = `MISSING 'skill': 必须提供技能名称
skill not found: 技能名称不存在；检查技能列表
unknown action: action 必须是 enable 或 disable`
	skillToolTips = `skill 在被禁用后不会影响已加载的上下文`
)

type SkillTool struct{ mgr *skill.Manager }

func (t *SkillTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: skillToolName,
		Desc: skillToolDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill":  {Type: schema.String, Desc: "Skill name", Required: true},
			"action": {Type: schema.String, Desc: "Action: enable (default) or disable"},
		}),
	}, nil
}

func (t *SkillTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input struct {
		Skill  string `json:"skill"`
		Action string `json:"action,omitempty"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("invalid skill arguments: %w. Expected JSON with 'skill' field.", err)
	}
	if input.Skill == "" {
		return "", fmt.Errorf("MISSING REQUIRED PARAMETER: 'skill' is required. Provide the skill name to enable or disable.")
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
	return "", fmt.Errorf("unknown action '%s'. Use 'enable' or 'disable'.", action)
}

var _ tool.InvokableTool = (*SkillTool)(nil)
