package agent

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

// injectSkills 将启用的技能作为独立消息注入到上下文
func (a *Agent) injectSkills(messageCtx *agentctx.Context) error {
	skills := a.skillManager.ListSkills()
	for _, skill := range skills {
		if skill.Enabled {
			skillMsg := &schema.Message{
				Role:    schema.System,
				Content: fmt.Sprintf("# Skill: %s\n%s\n\n%s", skill.Name, skill.Description, skill.Prompt),
			}
			if err := a.ctxManager.AddMessage(messageCtx, skillMsg); err != nil {
				return fmt.Errorf("failed to add skill %s: %w", skill.Name, err)
			}
		}
	}
	return nil
}

// HandleSkillCommand 处理技能相关命令
// 支持的命令:
//   - /skill list: 列出所有技能
//   - /skill enable <name>: 启用技能
//   - /skill disable <name>: 禁用技能
func (a *Agent) HandleSkillCommand(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid skill command, usage: /skill <list|enable|disable> [name]")
	}

	action := parts[1]
	switch action {
	case "list":
		return a.listSkills(), nil
	case "enable":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /skill enable <name>")
		}
		return a.enableSkill(parts[2])
	case "disable":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /skill disable <name>")
		}
		return a.disableSkill(parts[2])
	default:
		return "", fmt.Errorf("unknown skill command: %s", action)
	}
}

// listSkills 列出所有技能
func (a *Agent) listSkills() string {
	skills := a.skillManager.ListSkills()
	if len(skills) == 0 {
		return "No skills available"
	}

	var sb strings.Builder
	sb.WriteString("Available Skills:\n")
	for _, skill := range skills {
		status := "disabled"
		if skill.Enabled {
			status = "enabled"
		}
		sb.WriteString(fmt.Sprintf("  - %s [%s]: %s\n", skill.Name, status, skill.Description))
	}
	return sb.String()
}

// enableSkill 启用技能
func (a *Agent) enableSkill(name string) (string, error) {
	if err := a.skillManager.EnableSkill(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Skill '%s' enabled", name), nil
}

// disableSkill 禁用技能
func (a *Agent) disableSkill(name string) (string, error) {
	if err := a.skillManager.DisableSkill(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Skill '%s' disabled", name), nil
}
