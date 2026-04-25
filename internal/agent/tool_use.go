// Package agent 提供工具调用解析与执行相关功能。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/toolmeta"
)

type streamToolState struct {
	call       schema.ToolCall
	dispatched bool
}

type streamToolCollector struct {
	states []*streamToolState
	byID   map[string]int
}

type streamToolResult struct {
	idx    int
	tc     schema.ToolCall
	result string
	err    error
}

type streamToolRequest struct {
	idx int
	tc  schema.ToolCall
}

func newStreamToolCollector() *streamToolCollector {
	return &streamToolCollector{byID: make(map[string]int)}
}

func (c *streamToolCollector) Add(chunks []schema.ToolCall) []schema.ToolCall {
	if len(chunks) == 0 {
		return nil
	}

	for _, tc := range chunks {
		c.merge(tc)
	}

	ready := make([]schema.ToolCall, 0)
	for _, state := range c.states {
		if state.dispatched || !isRunnableToolCall(state.call) {
			continue
		}
		state.dispatched = true
		ready = append(ready, state.call)
	}

	return ready
}

func (c *streamToolCollector) merge(tc schema.ToolCall) {
	if tc.ID != "" {
		if idx, exists := c.byID[tc.ID]; exists {
			mergeToolCall(&c.states[idx].call, tc)
			return
		}

		if len(c.states) > 0 {
			last := c.states[len(c.states)-1]
			if last.call.ID == "" {
				mergeToolCall(&last.call, tc)
				c.byID[tc.ID] = len(c.states) - 1
				return
			}
		}

		c.states = append(c.states, &streamToolState{call: tc})
		c.byID[tc.ID] = len(c.states) - 1
		return
	}

	if len(c.states) == 0 {
		c.states = append(c.states, &streamToolState{call: tc})
		return
	}

	mergeToolCall(&c.states[len(c.states)-1].call, tc)
}

func (c *streamToolCollector) RunnableCalls() []schema.ToolCall {
	toolCalls := make([]schema.ToolCall, 0, len(c.states))
	for _, state := range c.states {
		if isRunnableToolCall(state.call) {
			toolCalls = append(toolCalls, state.call)
		}
	}
	return toolCalls
}

func mergeToolCall(dst *schema.ToolCall, src schema.ToolCall) {
	if dst.ID == "" && src.ID != "" {
		dst.ID = src.ID
	}
	if src.Function.Name != "" && dst.Function.Name == "" {
		dst.Function.Name = src.Function.Name
	}
	if src.Function.Arguments != "" {
		dst.Function.Arguments += src.Function.Arguments
	}
}

func isRunnableToolCall(tc schema.ToolCall) bool {
	if tc.ID == "" || tc.Function.Name == "" {
		return false
	}
	return isValidJSON(tc.Function.Arguments)
}

// exeTools 执行工具调用列表。
func (a *Agent) exeTools(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	logger.DebugTag("TOOL", "Executing %d tool(s)", len(toolCalls))

	var readOnlyCalls []schema.ToolCall
	var writeCalls []schema.ToolCall
	for _, tc := range toolCalls {
		if tc.Function.Name == "" {
			continue
		}
		if isReadOnlyToolCall(tc) {
			readOnlyCalls = append(readOnlyCalls, tc)
			continue
		}
		writeCalls = append(writeCalls, tc)
	}

	if len(readOnlyCalls) > 0 {
		if err := a.exeToolsConcurrent(ctx, messageCtx, readOnlyCalls); err != nil {
			return err
		}
	}

	for idx, tc := range writeCalls {
		result, execErr := a.executeToolCall(ctx, tc, idx, len(writeCalls), false)
		if err := a.addToolResultToContext(messageCtx, tc, result, execErr); err != nil {
			return err
		}
	}

	logger.DebugTag("TOOL", "All tools executed")
	return nil
}

func (a *Agent) exeToolsConcurrent(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	results := make(chan streamToolResult, len(toolCalls))

	for idx, tc := range toolCalls {
		go func(idx int, tc schema.ToolCall) {
			result, execErr := a.executeToolCall(ctx, tc, idx, len(toolCalls), true)
			results <- streamToolResult{idx: idx, tc: tc, result: result, err: execErr}
		}(idx, tc)
	}

	collectedResults := make([]streamToolResult, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		res := <-results
		collectedResults[res.idx] = res
	}

	for _, res := range collectedResults {
		if err := a.addToolResultToContext(messageCtx, res.tc, res.result, res.err); err != nil {
			return err
		}
	}

	return nil
}
func (a *Agent) executeToolCall(ctx context.Context, tc schema.ToolCall, idx, total int, concurrent bool) (string, error) {
	if tc.Function.Name == "" {
		logger.WarnTag("TOOL", "Skipping tool call with empty name, id=%s", tc.ID)
		return "", nil
	}

	logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, concurrent)
	logger.DebugTag("TOOL", "[%d/%d] name=%s id=%s", idx+1, total, tc.Function.Name, tc.ID)
	logger.DebugTag("TOOL", "  args: %s", tc.Function.Arguments)

	t := a.findTool(tc.Function.Name)
	if t == nil {
		logger.WarnTag("TOOL", "Not found: %s", tc.Function.Name)
		return "", fmt.Errorf("tool not found: %s", tc.Function.Name)
	}

	return a.invokeTool(ctx, t, tc)
}

func (a *Agent) invokeTool(ctx context.Context, t tool.BaseTool, tc schema.ToolCall) (string, error) {
	logger.DebugTag("TOOL", "Invoking: %s", tc.Function.Name)

	if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
		logger.DebugTag("TOOL", "Using EnhancedInvokableTool interface")
		toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}
		toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
		if err != nil {
			return "", err
		}
		return formatToolResult(toolResult), nil
	}

	if invokable, ok := t.(tool.InvokableTool); ok {
		logger.DebugTag("TOOL", "Using InvokableTool interface")
		return invokable.InvokableRun(ctx, tc.Function.Arguments)
	}

	return "", fmt.Errorf("tool %s is not invokable", tc.Function.Name)
}

func (a *Agent) addToolResultToContext(messageCtx *agentctx.Context, tc schema.ToolCall, result string, execErr error) error {
	if execErr != nil {
		logger.ErrorTag("TOOL", "Failed: %s, err=%v", tc.Function.Name, execErr)
		logger.PrintToolError(tc.Function.Name, tc.Function.Arguments, execErr)
		errMsg := schema.ToolMessage(formatToolExecutionError(tc, execErr), tc.ID)
		return a.ctxManager.AddMessage(messageCtx, errMsg)
	}

	logger.DebugTag("TOOL", "Success: %s", tc.Function.Name)
	logger.DebugTag("TOOL", "  result: %s", logger.TruncateString(result, 200))
	logger.PrintToolResult(tc.Function.Name, tc.Function.Arguments, result)

	return a.ctxManager.AddMessage(messageCtx, schema.ToolMessage(result, tc.ID))
}

func formatToolExecutionError(tc schema.ToolCall, execErr error) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tool execution failed: %v", execErr))
	if hint := toolFailureHint(tc); hint != "" {
		b.WriteString("\nSuggestion: ")
		b.WriteString(hint)
	}
	return b.String()
}

func toolFailureHint(tc schema.ToolCall) string {
	name := tc.Function.Name
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
		return "check the tool arguments and retry with an absolute path under the workspace; read the error text for the exact missing path or mismatch."
	case "exec_shell":
		return "check the shell command, quote paths with spaces, and prefer commands that operate inside the current workspace."
	case "task":
		return "use a valid task action: create, update, get, list, or delete, and include any required task id/title fields."
	case "skill":
		return "use an existing skill name and set action to enable or disable."
	}

	if meta, ok := toolmeta.Lookup(name); ok && meta.Category == toolmeta.CategoryMCP {
		return "check the remote tool arguments and server-specific requirements before retrying."
	}

	return "review the tool schema and retry with corrected arguments."
}

func isReadOnlyToolCall(tc schema.ToolCall) bool {
	if toolmeta.IsReadOnly(tc.Function.Name) {
		return true
	}

	if tc.Function.Name == "task.task" || tc.Function.Name == "task" {
		var input struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
			return false
		}
		return input.Action == "get" || input.Action == "list"
	}

	return false
}

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

func (a *Agent) findTool(name string) tool.BaseTool {
	return a.toolMap[name]
}

func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
