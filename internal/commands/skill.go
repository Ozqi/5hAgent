// Package commands 处理用户的斜杠命令
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
		return "", fmt.Errorf("usage: /skill <list|enable|disable> [name]")
	}

	action := parts[1]
	switch action {
	case "list":
		return listSkills(mgr), nil
	case "enable":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /skill enable <name>")
		}
		return enableSkill(mgr, parts[2])
	case "disable":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: /skill disable <name>")
		}
		return disableSkill(mgr, parts[2])
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
		status := "disabled"
		if s.Enabled {
			status = "enabled"
		}
		sb.WriteString(fmt.Sprintf("  - %s [%s]: %s\n", s.Name, status, s.Description))
	}
	return sb.String()
}

func enableSkill(mgr *skill.Manager, name string) (string, error) {
	if err := mgr.EnableSkill(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Skill '%s' enabled", name), nil
}

func disableSkill(mgr *skill.Manager, name string) (string, error) {
	if err := mgr.DisableSkill(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Skill '%s' disabled", name), nil
}
