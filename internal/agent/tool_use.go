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

// toolState 流式 ToolCall 收集状态
// 跟踪每个 ToolCall 的合并进度与分发状态
type toolState struct {
	call       schema.ToolCall
	dispatched bool
}

// streamToolCollector 流式 ToolCall 收集器
// 负责将 LLM 分片返回的 ToolCall 合并为完整调用
type streamToolCollector struct {
	states []*toolState
	byID   map[string]int
}

// execResult 工具执行结果
// 包含索引、原始调用、格式化结果与错误
type execResult struct {
	idx    int
	tc     schema.ToolCall
	result string
	err    error
}

// toolRequest 工具执行任务单元
// 包含在并发执行队列中的索引和 ToolCall
type toolRequest struct {
	idx int
	tc  schema.ToolCall
}

// newStreamToolCollector 创建流式 ToolCall 收集器
// 返回: 初始化好的 streamToolCollector 实例
func newStreamToolCollector() *streamToolCollector {
	return &streamToolCollector{byID: make(map[string]int)}
}

// Add 合并流式 ToolCall 分片，返回可分发的完整调用
// 参数:
//   - chunks: LLM 单次返回的 ToolCall 分片列表
//
// 返回: 已合并且未分发的完整 ToolCall 列表
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

// merge 将单条 ToolCall 合并到已有状态
// 参数:
//   - tc: 待合并的 ToolCall（可能只含 ID/Name/Arguments 其中几个字段）
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

// RunnableCalls 提取所有有效的 ToolCall（ID、Name、Arguments 均完整）
// 返回: 有效 ToolCall 列表，用于 ReAct 循环中的实际执行
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

// exeTools 执行工具调用调度
// 策略：只读工具并发执行，写入工具串行执行
// 参数:
//   - ctx: Go 标准上下文
//   - messageCtx: 消息上下文
//   - toolCalls: 待执行的 ToolCall 列表
//
// 返回: 所有工具执行完成后返回错误（若有）
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

// exeToolsPar 并发执行只读工具调用
// 参数:
//   - ctx: Go 标准上下文
//   - messageCtx: 消息上下文
//   - toolCalls: 只读工具调用列表
//
// 返回: 任意工具执行失败则返回错误
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

// exeToolCall 执行单个工具调用
// 参数:
//   - ctx: Go 标准上下文
//   - tc: 待执行的 ToolCall
//   - idx: 当前执行序号（从 0 开始）
//   - total: 本轮总工具数
//   - concurrent: 是否并发执行中
//
// 返回: 工具返回的格式化字符串和错误
func (a *Agent) exeToolCall(ctx context.Context, tc schema.ToolCall, idx, total int, concurrent bool) (string, error) {
	if tc.Function.Name == "" {
		logger.WarnTag("TOOL", "Skipping tool call with empty name, id=%s", tc.ID)
		return "", nil
	}

	// 记录工具调用
	if a.callbacks != nil {
		a.callbacks.OnToolStart(ctx, nil, &tool.CallbackInput{
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
			a.callbacks.OnToolError(ctx, nil, err)
		} else {
			a.callbacks.OnToolEnd(ctx, nil, &tool.CallbackOutput{Response: result})
		}
	}

	return result, err
}

// invokeTool 调用工具实例
// 参数:
//   - ctx: Go 标准上下文
//   - t: 工具实例
//   - tc: 原始 ToolCall
//
// 返回: 工具执行结果的格式化字符串和错误
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
// 参数:
//   - messageCtx: 消息上下文
//   - tc: 原始 ToolCall（用于关联 ID 和函数名）
//   - result: 工具返回的格式化字符串
//   - execErr: 工具执行错误（若有）
//
// 返回: 消息写入失败时返回错误
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

// formatToolErr 格式化工具执行错误为用户可读消息
// 参数:
//   - tc: 出错的 ToolCall
//   - execErr: 工具返回的错误
//
// 返回: 包含错误原因和操作提示的字符串
func formatToolErr(tc schema.ToolCall, execErr error) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tool execution failed: %v", execErr))
	if hint := toolHint(tc); hint != "" {
		b.WriteString("\nSuggestion: ")
		b.WriteString(hint)
	}
	return b.String()
}

// toolHint 根据工具名称返回操作提示
// 参数:
//   - tc: 待提示的 ToolCall
//
// 返回: 针对常见工具的纠错建议
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

// isReadOnly 判断工具调用是否为只读操作
// 参数:
//   - tc: 待判断的 ToolCall
//
// 返回: 只读返回 true，写入返回 false
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

// formatToolResult 将 schema.ToolResult 格式化为字符串
// 参数:
//   - toolResult: 工具返回的标准化结果结构
//
// 返回: 各 part 拼接成的字符串（text/image/audio/video/file）
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
// 参数:
//   - s: 待检查的字符串
//
// 返回: 合法返回 true，否则返回 false
func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
