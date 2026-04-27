// tool_use.go - 工具调用解析与执行
// 功能：流式 ToolCall 收集、并发/串行执行、结果格式化
// 主要类型：streamToolCollector, execResult, toolRequest
// 导出函数：exeTools, exeToolCall
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

type toolState struct {
	call       schema.ToolCall
	dispatched bool
}

type streamToolCollector struct {
	states []*toolState
	byID   map[string]int
}

type execResult struct {
	idx    int
	tc     schema.ToolCall
	result string
	err    error
}

type toolRequest struct {
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
		if state.dispatched {
			continue
		}
		tc := state.call
		if tc.ID == "" || tc.Function.Name == "" || !isValidJSON(tc.Function.Arguments) {
			continue
		}
		state.dispatched = true
		ready = append(ready, tc)
	}

	return ready
}

func (c *streamToolCollector) merge(tc schema.ToolCall) {
	merge := func(dst *schema.ToolCall, src schema.ToolCall) {
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

	if tc.ID != "" {
		if idx, exists := c.byID[tc.ID]; exists {
			merge(&c.states[idx].call, tc)
			return
		}

		if len(c.states) > 0 {
			last := c.states[len(c.states)-1]
			if last.call.ID == "" {
				merge(&last.call, tc)
				c.byID[tc.ID] = len(c.states) - 1
				return
			}
		}

		c.states = append(c.states, &toolState{call: tc})
		c.byID[tc.ID] = len(c.states) - 1
		return
	}

	if len(c.states) == 0 {
		c.states = append(c.states, &toolState{call: tc})
		return
	}

	merge(&c.states[len(c.states)-1].call, tc)
}

func (c *streamToolCollector) RunnableCalls() []schema.ToolCall {
	toolCalls := make([]schema.ToolCall, 0, len(c.states))
	for _, state := range c.states {
		tc := state.call
		if tc.ID != "" && tc.Function.Name != "" && isValidJSON(tc.Function.Arguments) {
			toolCalls = append(toolCalls, tc)
		}
	}
	return toolCalls
}

// exeTools executes tool calls: read-only concurrent, write serial.
func (a *Agent) exeTools(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	logger.DebugTag("TOOL", "Executing %d tool(s)", len(toolCalls))

	var readOnlyCalls []schema.ToolCall
	var writeCalls []schema.ToolCall
	for _, tc := range toolCalls {
		if tc.Function.Name == "" {
			continue
		}
		if isReadOnly(tc) {
			readOnlyCalls = append(readOnlyCalls, tc)
		} else {
			writeCalls = append(writeCalls, tc)
		}
	}

	if len(readOnlyCalls) > 0 {
		if err := a.exeToolsPar(ctx, messageCtx, readOnlyCalls); err != nil {
			return err
		}
	}

	for idx, tc := range writeCalls {
		result, execErr := a.exeToolCall(ctx, tc, idx, len(writeCalls), false)
		if err := a.addToolResult(messageCtx, tc, result, execErr); err != nil {
			return err
		}
	}

	logger.DebugTag("TOOL", "All tools executed")
	return nil
}

func (a *Agent) exeToolsPar(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	results := make(chan execResult, len(toolCalls))

	for idx, tc := range toolCalls {
		go func(idx int, tc schema.ToolCall) {
			result, execErr := a.exeToolCall(ctx, tc, idx, len(toolCalls), true)
			results <- execResult{idx: idx, tc: tc, result: result, err: execErr}
		}(idx, tc)
	}

	collectedResults := make([]execResult, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		res := <-results
		collectedResults[res.idx] = res
	}

	for _, res := range collectedResults {
		if err := a.addToolResult(messageCtx, res.tc, res.result, res.err); err != nil {
			return err
		}
	}

	return nil
}

func (a *Agent) exeToolCall(ctx context.Context, tc schema.ToolCall, idx, total int, concurrent bool) (string, error) {
	if tc.Function.Name == "" {
		logger.WarnTag("TOOL", "Skipping tool call with empty name, id=%s", tc.ID)
		return "", nil
	}

	logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, concurrent)
	logger.DebugTag("TOOL", "[%d/%d] name=%s id=%s", idx+1, total, tc.Function.Name, tc.ID)
	logger.DebugTag("TOOL", "  args: %s", tc.Function.Arguments)

	t := a.toolMap[tc.Function.Name]
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

func formatToolErr(tc schema.ToolCall, execErr error) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tool execution failed: %v", execErr))
	if hint := toolHint(tc); hint != "" {
		b.WriteString("\nSuggestion: ")
		b.WriteString(hint)
	}
	return b.String()
}

func toolHint(tc schema.ToolCall) string {
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

func isReadOnly(tc schema.ToolCall) bool {
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

func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
