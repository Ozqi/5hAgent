// debug_request.go - 将 Agent 层可见的逻辑 LLM 请求写入现有 debug log。
package agent

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/logger"
)

type debugLLMRequest struct {
	Model        string         `json:"model"`
	ModelOptions string         `json:"model_options"`
	Messages     []debugMessage `json:"messages"`
}

type debugMessage struct {
	Role       schema.RoleType `json:"role"`
	Name       string          `json:"name,omitempty"`
	Content    string          `json:"content"`
	ToolCalls  []debugToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
}

type debugToolCall struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (a *Agent) logLLMRequest(messages []*schema.Message, optionCount int) {
	data, err := buildDebugLLMRequest(messages, a.modelRef, a.debugSecret, optionCount)
	if err != nil {
		logger.DebugTag("LLMREQ", "marshal failed: %v", err)
		return
	}
	logger.DebugTag("LLMREQ", "\n%s", data)
}

func buildDebugLLMRequest(messages []*schema.Message, modelRef string, secret string, optionCount int) ([]byte, error) {
	request := debugLLMRequest{
		Model:        modelRef,
		ModelOptions: "unavailable",
		Messages:     make([]debugMessage, 0, len(messages)),
	}
	if optionCount == 0 {
		request.ModelOptions = "none"
	}
	for _, message := range messages {
		if message == nil {
			continue
		}
		item := debugMessage{
			Role:       message.Role,
			Name:       message.Name,
			Content:    redactDebugSecret(message.Content, secret),
			ToolCallID: message.ToolCallID,
			ToolName:   message.ToolName,
		}
		for _, call := range message.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, debugToolCall{
				ID:        call.ID,
				Type:      call.Type,
				Name:      call.Function.Name,
				Arguments: redactDebugSecret(call.Function.Arguments, secret),
			})
		}
		request.Messages = append(request.Messages, item)
	}
	return json.MarshalIndent(request, "", "  ")
}

func redactDebugSecret(text string, secret string) string {
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "[REDACTED]")
}
