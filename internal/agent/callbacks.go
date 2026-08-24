package agent

import (
	"context"

	"github.com/Ozqi/walle/internal/logger"
	"github.com/Ozqi/walle/internal/utils"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// AgentCallbacks Eino Callback 处理器
// 统一处理模型和工具的调用日志
type AgentCallbacks struct {
	debug       bool
	tokenBudget *utils.TokenBudget
}

// NewAgentCallbacks 创建 Callback 处理器
func NewAgentCallbacks(debug bool, tokenBudget *utils.TokenBudget) *AgentCallbacks {
	return &AgentCallbacks{
		debug:       debug,
		tokenBudget: tokenBudget,
	}
}

// 区域：模型回调

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
	if output.TokenUsage != nil {
		c.tokenBudget.AddUsage(output.TokenUsage)
	}
	if !c.debug {
		return ctx
	}
	if output.TokenUsage == nil {
		logger.DebugTag("LLM", "End")
		return ctx
	}
	logger.DebugTag("LLM", "End: prompt=%d completion=%d total=%d (session=%d)",
		output.TokenUsage.PromptTokens,
		output.TokenUsage.CompletionTokens,
		output.TokenUsage.TotalTokens,
		c.tokenBudget.SessionTotal())
	return ctx
}

// OnModelError 模型调用错误
func (c *AgentCallbacks) OnModelError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	logger.ErrorTag("LLM", "Error: %v", err)
	return ctx
}

// 区域结束

// 区域：工具回调

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

// 区域结束

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
