package commands

import (
	"fmt"
	"strings"

	"github.com/Ozqi/walle/internal/skill"
)

// HandleSkill 校验并分派 /skill 子命令，返回适合终端展示的技能信息。
func HandleSkill(cmd string, mgr *skill.Manager) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return "", fmt.Errorf("usage: /skill <list|get|reload> [name]")
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
	case "reload":
		if err := mgr.ReloadSkills(); err != nil {
			return "", err
		}
		return fmt.Sprintf("Reloaded %d skills; current context keeps already injected skill messages", len(mgr.ListSkills())), nil
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
		sb.WriteString(fmt.Sprintf("  - %s [%s]\n    path: %s\n    %s\n", s.Name, s.Scope, s.Path, s.Description))
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
