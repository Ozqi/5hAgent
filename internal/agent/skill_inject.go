package agent

import (
	"fmt"

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
