// Package agent 提供工具执行相关功能
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
)

// 只读工具列表（支持并发执行）
var readOnlyTools = map[string]bool{
	"read_file": true,
	"glob":      true,
	"grep":      true,
	"list_dir":  true,
	"task_get":  true,
	"task_list": true,
}

// exeTools 执行工具调用列表
// 参数:
//   - ctx: 上下文
//   - messageCtx: 消息上下文
//   - toolCalls: 工具调用列表
//
// 返回: 可能的错误
// 功能:
//  1. 分类工具（只读 vs 写入）
//  2. 并发执行只读工具
//  3. 串行执行写工具
//  4. 将结果添加到消息上下文
func (a *Agent) exeTools(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	logger.DebugTag("TOOL", "Executing %d tool(s)", len(toolCalls))

	// 分类工具调用
	var readOnlyCalls []schema.ToolCall
	var writeCalls []schema.ToolCall

	for _, tc := range toolCalls {
		if tc.Function.Name == "" {
			continue
		}
		if readOnlyTools[tc.Function.Name] {
			readOnlyCalls = append(readOnlyCalls, tc)
		} else {
			writeCalls = append(writeCalls, tc)
		}
	}

	// 并发执行只读工具
	if len(readOnlyCalls) > 0 {
		if err := a.exeToolsConcurrent(ctx, messageCtx, readOnlyCalls); err != nil {
			return err
		}
	}

	// 串行执行写工具
	for idx, tc := range writeCalls {
		if err := a.executeSingleTool(ctx, messageCtx, tc, idx, len(writeCalls), false); err != nil {
			return err
		}
	}

	logger.DebugTag("TOOL", "All tools executed")
	return nil
}

// exeToolsConcurrent 并发执行只读工具
// 参数:
//   - ctx: 上下文
//   - messageCtx: 消息上下文
//   - toolCalls: 工具调用列表
//
// 返回: 可能的错误
// 功能:
//  1. 使用 goroutine 并发执行所有工具
//  2. 使用 channel 收集结果
//  3. 按原始顺序添加结果到上下文
func (a *Agent) exeToolsConcurrent(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	type toolResult struct {
		idx    int
		tc     schema.ToolCall
		result string
		err    error
	}

	results := make(chan toolResult, len(toolCalls))

	// 并发执行
	for idx, tc := range toolCalls {
		go func(idx int, tc schema.ToolCall) {
			// 显示工具执行提示
			logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, true)
			logger.DebugTag("TOOL", "[%d/%d] name=%s id=%s (concurrent)", idx+1, len(toolCalls), tc.Function.Name, tc.ID)

			// 查找工具
			t := a.findTool(tc.Function.Name)
			if t == nil {
				results <- toolResult{idx: idx, tc: tc, err: fmt.Errorf("tool not found: %s", tc.Function.Name)}
				return
			}

			// 执行工具
			result, execErr := a.invokeTool(ctx, t, tc)
			results <- toolResult{idx: idx, tc: tc, result: result, err: execErr}
		}(idx, tc)
	}

	// 收集结果
	collectedResults := make([]toolResult, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		res := <-results
		collectedResults[res.idx] = res
	}

	// 按顺序添加结果到上下文
	for _, res := range collectedResults {
		if err := a.addToolResultToContext(messageCtx, res.tc, res.result, res.err); err != nil {
			return err
		}
	}

	return nil
}

// executeToolStreaming 在流式输出过程中执行单个工具
// 这个函数会在 goroutine 中异步调用，避免阻塞流式输出
// 参数:
//   - ctx: 上下文
//   - messageCtx: 消息上下文
//   - toolCall: 工具调用
//
// 功能: 异步执行工具，不返回错误（错误会记录到日志）
func (a *Agent) executeToolStreaming(ctx context.Context, messageCtx *agentctx.Context, toolCall *schema.ToolCall) {
	logger.DebugTag("STREAM-TOOL", "Executing tool: id=%s name=%s", toolCall.ID, toolCall.Function.Name)

	// 显示工具执行提示
	logger.PrintToolCall(toolCall.Function.Name, toolCall.Function.Arguments, false)

	// 查找工具
	t := a.findTool(toolCall.Function.Name)
	if t == nil {
		logger.WarnTag("STREAM-TOOL", "Tool not found: %s", toolCall.Function.Name)
		return
	}

	// 执行工具
	result, execErr := a.invokeTool(ctx, t, *toolCall)

	// 添加结果到上下文（忽略错误，因为是异步执行）
	_ = a.addToolResultToContext(messageCtx, *toolCall, result, execErr)
}

// executeSingleTool 执行单个工具（串行）
// 参数:
//   - ctx: 上下文
//   - messageCtx: 消息上下文
//   - tc: 工具调用
//   - idx: 当前索引
//   - total: 总数
//   - concurrent: 是否并发执行
//
// 返回: 可能的错误
func (a *Agent) executeSingleTool(ctx context.Context, messageCtx *agentctx.Context, tc schema.ToolCall, idx, total int, concurrent bool) error {
	// 跳过无效的 ToolCall
	if tc.Function.Name == "" {
		logger.WarnTag("TOOL", "Skipping tool call with empty name, id=%s", tc.ID)
		return nil
	}

	// 显示工具执行提示
	logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, concurrent)
	logger.DebugTag("TOOL", "[%d/%d] name=%s id=%s", idx+1, total, tc.Function.Name, tc.ID)
	logger.DebugTag("TOOL", "  args: %s", tc.Function.Arguments)

	// 查找工具
	t := a.findTool(tc.Function.Name)
	if t == nil {
		logger.WarnTag("TOOL", "Not found: %s", tc.Function.Name)
		errMsg := schema.ToolMessage(
			fmt.Sprintf("tool not found: %s", tc.Function.Name),
			tc.ID,
		)
		return a.ctxManager.AddMessage(messageCtx, errMsg)
	}

	// 执行工具
	result, execErr := a.invokeTool(ctx, t, tc)

	// 添加结果到上下文
	return a.addToolResultToContext(messageCtx, tc, result, execErr)
}

// invokeTool 调用工具（核心逻辑）
// 参数:
//   - ctx: 上下文
//   - t: 工具实例
//   - tc: 工具调用
//
// 返回: 结果字符串和可能的错误
// 功能:
//  1. 尝试 EnhancedInvokableTool 接口（支持富媒体）
//  2. 降级到 InvokableTool 接口（仅文本）
//  3. 都不支持则返回错误
func (a *Agent) invokeTool(ctx context.Context, t tool.BaseTool, tc schema.ToolCall) (string, error) {
	logger.DebugTag("TOOL", "Invoking: %s", tc.Function.Name)

	// 尝试 EnhancedInvokableTool (返回 *schema.ToolResult)
	if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
		logger.DebugTag("TOOL", "Using EnhancedInvokableTool interface")
		toolArg := &schema.ToolArgument{
			Text: tc.Function.Arguments,
		}
		toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
		if err != nil {
			return "", err
		}
		// 将 ToolResult 转换为字符串
		return formatToolResult(toolResult), nil
	}

	// 尝试 InvokableTool (返回 string)
	if invokable, ok := t.(tool.InvokableTool); ok {
		logger.DebugTag("TOOL", "Using InvokableTool interface")
		return invokable.InvokableRun(ctx, tc.Function.Arguments)
	}

	// 工具不支持任何可调用接口
	return "", fmt.Errorf("tool %s is not invokable", tc.Function.Name)
}

// addToolResultToContext 将工具结果添加到上下文
// 参数:
//   - messageCtx: 消息上下文
//   - tc: 工具调用
//   - result: 结果字符串
//   - execErr: 执行错误
//
// 返回: 可能的错误
// 功能: 根据执行结果添加成功或错误消息到上下文
func (a *Agent) addToolResultToContext(messageCtx *agentctx.Context, tc schema.ToolCall, result string, execErr error) error {
	if execErr != nil {
		logger.ErrorTag("TOOL", "Failed: %s, err=%v", tc.Function.Name, execErr)
		logger.PrintToolError(execErr)
		errMsg := schema.ToolMessage(
			fmt.Sprintf("tool execution failed: %v", execErr),
			tc.ID,
		)
		return a.ctxManager.AddMessage(messageCtx, errMsg)
	}

	// 工具执行成功
	logger.DebugTag("TOOL", "Success: %s", tc.Function.Name)
	logger.DebugTag("TOOL", "  result: %s", logger.TruncateString(result, 200))
	logger.PrintToolResult(result)

	resultMsg := schema.ToolMessage(result, tc.ID)
	return a.ctxManager.AddMessage(messageCtx, resultMsg)
}

// formatToolResult 将 ToolResult 转换为字符串
// 参数:
//   - toolResult: Eino 的 ToolResult 结构
//
// 返回: 格式化后的字符串
// 功能: 将 ToolResult 的所有 Parts 合并为一个字符串
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
			// 图片类型，显示占位符
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

// findTool 查找工具
// 参数:
//   - name: 工具名称
//
// 返回: 工具实例或 nil
// 功能: 从工具映射表中查找指定名称的工具（O(1)复杂度）
func (a *Agent) findTool(name string) tool.BaseTool {
	return a.toolMap[name]
}

// isValidJSON 检查字符串是否是有效的 JSON
func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
