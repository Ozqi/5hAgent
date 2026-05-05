// tool_use.go - 工具调用解析与执行
// 功能：流式 ToolCall 收集、单工具执行、结果格式化
// 主要类型：toolCollector, execResult, toolRequest
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/tools"
)

// toolCallState 工具调用收集状态
type toolCallState struct {
	tc         schema.ToolCall
	dispatched bool
}

// toolCollector 流式工具调用收集器
// 合并 LLM 分片返回的 ToolCall
type toolCollector struct {
	states map[int]*toolCallState // index -> state
	byID   map[string]int         // id -> index
}

// newToolCollector 创建收集器
func newToolCollector() *toolCollector {
	return &toolCollector{
		states: make(map[int]*toolCallState),
		byID:   make(map[string]int),
	}
}

// Add 添加分片，返回可执行的调用
func (c *toolCollector) Add(chunks []schema.ToolCall) []schema.ToolCall {
	if len(chunks) == 0 {
		return nil
	}

	for _, tc := range chunks {
		logger.DebugTag("COLL", "Add chunk: id=%s name=%s args=%q", tc.ID, tc.Function.Name, logger.TruncateString(tc.Function.Arguments, 100))
		c.merge(tc)
	}
	return c.extractReady()
}

// PendingRunnableCalls returns complete calls that were not dispatched while streaming.
// Empty arguments are normalized to {} for no-argument tools after the stream ends.
func (c *toolCollector) PendingRunnableCalls() []schema.ToolCall {
	return c.runnableCalls(false, true)
}

// merge 合并单个 ToolCall
func (c *toolCollector) merge(tc schema.ToolCall) {
	idx := 0
	if tc.Index != nil {
		idx = *tc.Index
	}

	state, ok := c.states[idx]
	if !ok {
		state = &toolCallState{}
		c.states[idx] = state
	}

	// 合并字段
	if state.tc.ID == "" && tc.ID != "" {
		state.tc.ID = tc.ID
	}
	if state.tc.Function.Name == "" && tc.Function.Name != "" {
		state.tc.Function.Name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		state.tc.Function.Arguments += tc.Function.Arguments
	}

	// 当 ID、Name、Arguments 都已齐全但 Arguments 为空时，
	// 标准化为 "{}"，让流式阶段也能识别为完整调用
	if state.tc.ID != "" && state.tc.Function.Name != "" && state.tc.Function.Arguments == "" {
		state.tc.Function.Arguments = "{}"
	}

	// 记录 ID 映射
	if tc.ID != "" {
		c.byID[tc.ID] = idx
	}
}

// extractReady 提取已完成的调用
func (c *toolCollector) extractReady() []schema.ToolCall {
	return c.runnableCalls(false, false)
}

func (c *toolCollector) runnableCalls(includeDispatched bool, allowEmptyArguments bool) []schema.ToolCall {
	var ready []schema.ToolCall
	for idx := range c.states {
		state := c.states[idx]
		if state.dispatched && !includeDispatched {
			continue
		}
		tc := state.tc
		// Streaming cannot distinguish "no arguments" from "arguments not arrived yet" until EOF.
		if allowEmptyArguments && tc.Function.Arguments == "" {
			tc.Function.Arguments = "{}"
			state.tc.Function.Arguments = tc.Function.Arguments
		}
		if tc.ID == "" || tc.Function.Name == "" || tc.Function.Arguments == "" || !isValidJSON(tc.Function.Arguments) {
			continue
		}
		state.dispatched = true
		ready = append(ready, tc)
	}
	return ready
}

// RunnableCalls 返回所有有效调用
func (c *toolCollector) RunnableCalls() []schema.ToolCall {
	return c.runnableCalls(true, true)
}

// execResult 工具执行结果
type execResult struct {
	idx    int
	tc     schema.ToolCall
	result string
	err    error
}

// toolRequest 工具执行任务单元
type toolRequest struct {
	idx int
	tc  schema.ToolCall
}

// exeToolCall 执行单个工具调用
func (a *Agent) exeToolCall(ctx context.Context, tc schema.ToolCall, idx, total int, concurrent bool) (string, error) {
	if tc.Function.Name == "" {
		logger.WarnTag("TOOL", "Skipping tool call with empty name, id=%s", tc.ID)
		return "", nil
	}

	// 记录工具调用
	runInfo := &callbacks.RunInfo{Name: tc.Function.Name}
	if a.callbacks != nil {
		a.callbacks.OnToolStart(ctx, runInfo, &tool.CallbackInput{
			ArgumentsInJSON: tc.Function.Arguments,
		})
	}

	logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, concurrent)

	t := a.toolMap[tc.Function.Name]
	if t == nil {
		logger.WarnTag("TOOL", "Not found: %s", tc.Function.Name)
		return "", fmt.Errorf("tool not found: %s", tc.Function.Name)
	}

	result, err := a.invokeTool(ctx, t, tc)

	// 记录结果
	if a.callbacks != nil {
		if err != nil {
			a.callbacks.OnToolError(ctx, runInfo, err)
		} else {
			a.callbacks.OnToolEnd(ctx, runInfo, &tool.CallbackOutput{Response: result})
		}
	}

	return result, err
}

// invokeTool 调用工具实例
func (a *Agent) invokeTool(ctx context.Context, t tool.BaseTool, tc schema.ToolCall) (string, error) {
	if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
		toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}
		toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
		if err != nil {
			return "", err
		}
		return formatToolResult(toolResult), nil
	}

	if invokable, ok := t.(tool.InvokableTool); ok {
		return invokable.InvokableRun(ctx, tc.Function.Arguments)
	}

	return "", fmt.Errorf("tool %s is not invokable", tc.Function.Name)
}

// addToolResult 将工具执行结果写入消息上下文
func (a *Agent) addToolResult(messageCtx *agentctx.Context, tc schema.ToolCall, result string, execErr error) error {
	if execErr != nil {
		logger.ErrorTag("TOOL", "Failed: %s, err=%v", tc.Function.Name, execErr)
		logger.PrintToolError(tc.Function.Name, tc.Function.Arguments, execErr)
		errMsg := schema.ToolMessage(formatToolErr(tc, execErr), tc.ID)
		return a.ctxManager.AddMessage(messageCtx, errMsg)
	}

	logger.DebugTag("TOOL", "Success: %s", tc.Function.Name)
	logger.DebugTag("TOOL", "  result: %s", logger.TruncateString(result, 200))
	logger.PrintToolResult(tc.Function.Name, tc.Function.Arguments, result)

	return a.ctxManager.AddMessage(messageCtx, schema.ToolMessage(result, tc.ID))
}

// formatToolErr 格式化工具执行错误
func formatToolErr(tc schema.ToolCall, execErr error) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tool execution failed: %v", execErr))
	if strings.TrimSpace(tc.Function.Arguments) != "" {
		b.WriteString("\nTool arguments sent by model: ")
		b.WriteString(tc.Function.Arguments)
	}
	if hint := toolHint(tc); hint != "" {
		b.WriteString("\nSuggestion: ")
		b.WriteString(hint)
	}
	return b.String()
}

// toolHint 根据工具名称返回操作提示
func toolHint(tc schema.ToolCall) string {
	name := tc.Function.Name
	display := tools.DisplayName(name)
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
		return "use exact task action values only: create/update/get/list/delete/archive/reopen. To finish a task use {\"action\":\"update\",\"id\":\"...\",\"status\":\"completed\"}; create requires id/title/description"
	case "skill":
		return "use an existing skill name and set action to enable or disable"
	}

	if meta, ok := tools.Lookup(name); ok && meta.Category == tools.CategoryMCP {
		// MCP 错误已在 mcp_tool.go 返回具体信息，不添加通用提示干扰
		return ""
	}

	return "review the tool schema and retry with corrected arguments"
}

// formatToolResult 将 schema.ToolResult 格式化为字符串
func formatToolResult(toolResult *schema.ToolResult) string {
	if toolResult == nil || len(toolResult.Parts) == 0 {
		return ""
	}

	var parts []string
	for _, part := range toolResult.Parts {
		switch part.Type {
		case schema.ToolPartTypeText:
			parts = append(parts, part.Text)
		case schema.ToolPartTypeImage:
			parts = append(parts, "[Image]")
		case schema.ToolPartTypeAudio:
			parts = append(parts, "[Audio]")
		case schema.ToolPartTypeVideo:
			parts = append(parts, "[Video]")
		case schema.ToolPartTypeFile:
			parts = append(parts, "[File]")
		}
	}

	return strings.Join(parts, "\n")
}

// isValidJSON 检查字符串是否为合法 JSON
func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
