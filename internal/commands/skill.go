// skill.go - /skill 命令处理
// 功能：解析 /skill 命令（list/get），查看 Agent 启动时加载的技能快照
// 导出函数：HandleSkill, listSkills, getSkill
package commands

import (
	"fmt"
	"strings"

	"github.com/lzq/5hAgent/internal/skill"
)

// HandleSkill 处理 /skill 命令
func HandleSkill(cmd string, mgr *skill.Manager) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return "", fmt.Errorf("usage: /skill <list|get> [name]")
	}

	action := parts[1]
	switch action {
	case "list":
		return listSkills(mgr), nil
	case "get":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /skill get <name>")
		}
		return getSkill(mgr, parts[2])
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func listSkills(mgr *skill.Manager) string {
	skills := mgr.ListSkills()
	if len(skills) == 0 {
		return "No skills available"
	}

	var sb strings.Builder
	sb.WriteString("Available Skills:\n")
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("  - %s [%s]: %s\n", s.Name, s.Scope, s.Description))
	}
	return sb.String()
}

func getSkill(mgr *skill.Manager, name string) (string, error) {
	s, ok := mgr.GetSkill(name)
	if !ok {
		return "", fmt.Errorf("skill not found: %s", name)
	}
	return fmt.Sprintf("# Skill: %s\n\nscope: %s\npath: %s\n\n%s", s.Name, s.Scope, s.Path, s.Content), nil
}
