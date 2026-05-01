// callbacks.go - Eino Callback 实现
// 功能: 统一的日志、监控、调试输出，替代分散的 logger.DebugTag
// 导出: AgentCallbacks, NewAgentCallbacks, GetTokenUsage
package agent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/toolmeta"
)

// AgentCallbacks Eino Callback 处理器
// 统一处理模型和工具的调用日志
type AgentCallbacks struct {
	debug       bool           // 是否启用调试
	tokenBudget *tokenCounter  // 累计 token
}

// tokenCounter 累计 token 计数
type tokenCounter struct {
	total int
}

// newTokenCounter 创建 token 计数器
func newTokenCounter() *tokenCounter {
	return &tokenCounter{}
}

// add 添加 token
func (c *tokenCounter) add(usage *model.TokenUsage) {
	if usage == nil {
		return
	}
	if usage.TotalTokens > 0 {
		c.total += usage.TotalTokens
	} else {
		c.total += usage.PromptTokens + usage.CompletionTokens
	}
}

// NewAgentCallbacks 创建 Callback 处理器
func NewAgentCallbacks(debug bool) *AgentCallbacks {
	return &AgentCallbacks{
		debug:       debug,
		tokenBudget: newTokenCounter(),
	}
}

// GetTokenUsage 获取累计 token
func (c *AgentCallbacks) GetTokenUsage() int {
	return c.tokenBudget.total
}

// region Model Callback

// OnModelStart 模型开始调用
func (c *AgentCallbacks) OnModelStart(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
	if !c.debug {
		return ctx
	}
	logger.DebugTag("LLM", "Start: messages=%d", len(input.Messages))
	return ctx
}

// OnModelEnd 模型调用结束
func (c *AgentCallbacks) OnModelEnd(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
	if !c.debug {
		return ctx
	}
	if output.TokenUsage != nil {
		c.tokenBudget.add(output.TokenUsage)
		logger.DebugTag("LLM", "End: prompt=%d completion=%d total=%d (session=%d)",
			output.TokenUsage.PromptTokens,
			output.TokenUsage.CompletionTokens,
			output.TokenUsage.TotalTokens,
			c.tokenBudget.total)
	} else {
		logger.DebugTag("LLM", "End")
	}
	return ctx
}

// OnModelError 模型调用错误
func (c *AgentCallbacks) OnModelError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	logger.ErrorTag("LLM", "Error: %v", err)
	return ctx
}

// endregion

// region Tool Callback

// OnToolStart 工具开始调用
func (c *AgentCallbacks) OnToolStart(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
	if !c.debug {
		return ctx
	}
	logger.DebugTag("TOOL", "Start: name=%s", info.Name)
	if c.debug && len(input.ArgumentsInJSON) < 200 {
		logger.DebugTag("TOOL", "  args: %s", input.ArgumentsInJSON)
	}
	return ctx
}

// OnToolEnd 工具调用结束
func (c *AgentCallbacks) OnToolEnd(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
	if !c.debug {
		return ctx
	}
	if output.Response != "" {
		preview := logger.TruncateString(output.Response, 100)
		logger.DebugTag("TOOL", "End: name=%s result=%s", info.Name, preview)
	} else {
		logger.DebugTag("TOOL", "End: name=%s", info.Name)
	}
	return ctx
}

// OnToolError 工具调用错误
func (c *AgentCallbacks) OnToolError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	logger.ErrorTag("TOOL", "Error: name=%s err=%v", info.Name, err)
	return ctx
}

// endregion

// region 流式 ToolCall 处理（用于 RunStream）

// LogToolCall 记录工具调用
func (c *AgentCallbacks) LogToolCall(idx int, tc schema.ToolCall) {
	if !c.debug {
		return
	}
	logger.DebugTag("STREAM", "  [%d] id=%s name=%s",
		idx, tc.ID, tc.Function.Name)
}

// LogToolCalls 记录多个工具调用
func (c *AgentCallbacks) LogToolCalls(calls []schema.ToolCall) {
	if !c.debug || len(calls) == 0 {
		return
	}
	logger.DebugTag("TOOL", "Tool calls: %d", len(calls))
	for i, tc := range calls {
		c.LogToolCall(i, tc)
	}
}

// LogChunk 记录流式 chunk
func (c *AgentCallbacks) LogChunk(chunk *schema.Message, idx int) {
	if !c.debug || chunk == nil {
		return
	}
	if len(chunk.ToolCalls) > 0 {
		logger.DebugTag("STREAM", "Chunk#%d: tools=%d", idx, len(chunk.ToolCalls))
		for i, tc := range chunk.ToolCalls {
			logger.DebugTag("STREAM", "  [%d] id=%s name=%s",
				i, tc.ID, tc.Function.Name)
		}
	}
}

// endregion

// region 工具提示（保留 toolHint 逻辑）

// ToolHint 返回工具操作的纠错建议
func (c *AgentCallbacks) ToolHint(name, args string) string {
	display := toolmeta.DisplayName(name)
	if strings.HasPrefix(display, "base.") {
		display = strings.TrimPrefix(display, "base.")
	}
	if strings.HasPrefix(display, "task.") {
		display = strings.TrimPrefix(display, "task.")
	}
	if strings.HasPrefix(display, "skill.") {
		display = strings.TrimPrefix(display, "skill.")
	}

	switch display {
	case "read_file", "write_file", "edit", "glob", "grep", "list_dir":
		return "check the tool arguments and retry with an absolute path under the workspace"
	case "exec_shell":
		return "check the shell command, quote paths with spaces, prefer commands inside workspace"
	case "task":
		return "use a valid task action: create, update, get, list, or delete"
	case "skill":
		return "use an existing skill name and set action to enable or disable"
	}

	if meta, ok := toolmeta.Lookup(name); ok && meta.Category == toolmeta.CategoryMCP {
		return "check the remote tool arguments and server-specific requirements"
	}

	return "review the tool schema and retry with corrected arguments"
}

// endregion
