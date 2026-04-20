// Package agent 提供 AI Agent 的核心实现
package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/miniAgent/internal/context"
	"github.com/lzq/miniAgent/internal/logger"
)

// Agent AI Agent 核心结构体
// 负责协调 LLM、工具、上下文管理器
type Agent struct {
	// 核心组件
	model   model.ToolCallingChatModel // LLM 模型
	tools   []tool.BaseTool            // 工具列表
	toolMap map[string]tool.BaseTool   // 工具名称映射表（优化查找）

	// 配置
	config *Config // Agent 配置
	// 状态
	state *State // Agent 状态
	// 上下文管理器
	ctxManager *agentctx.Manager // 复用Manager实例
}

// Config Agent 配置
type Config struct {
	Name         string // Agent 名称
	MaxTurns     int    // 最大对话轮数
	Debug        bool   // 是否启用调试
	SystemPrompt string // 系统提示词
}

// State Agent 运行状态
type State struct {
	CurrentTurn int  // 当前轮数
	IsRunning   bool // 是否运行中
}

// NewAgent 创建新的 Agent
// 参数:
//   - model: LLM 模型
//   - tools: 工具列表
//   - config: Agent 配置（包含系统提示词）
//
// 返回: Agent 实例和可能的错误
// 功能:
//  1. 初始化 Agent 结构体
//  2. 初始化 Agent 状态
//  3. 构建工具名称映射表
func NewAgent(model model.ToolCallingChatModel, tools []tool.BaseTool, config *Config) (*Agent, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// 构建工具映射表
	toolMap := make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		toolMap[info.Name] = t
	}

	return &Agent{
		model:      model,
		tools:      tools,
		toolMap:    toolMap,
		config:     config,
		ctxManager: agentctx.NewManager(),
		state: &State{
			CurrentTurn: 0,
			IsRunning:   false,
		},
	}, nil
}

// Run 运行 Agent，处理用户输入
// 参数:
//   - ctx: Go 标准上下文（超时、取消控制）
//   - messageCtx: 消息上下文（对话历史管理）
//   - input: 用户输入
//
// 返回: Agent 响应内容和可能的错误
// 功能: 实现 ReAct 循环架构
//  1. 注入SystemPrompt（如果messageCtx为空）
//  2. 添加用户消息到 messageCtx
//  3. 进入 ReAct 循环 (最多 MaxTurns 轮):
//     a. 从 messageCtx 获取所有消息
//     b. 调用 LLM 生成响应 (Reasoning)
//     c. 检查响应中是否有工具调用
//     d. 如果有工具调用:
//     - 执行工具 (Acting)
//     - 将工具结果添加到 messageCtx
//     - 继续循环
//     e. 如果没有工具调用:
//     - 将 LLM 响应添加到 messageCtx
//     - 返回响应内容
//  4. 如果达到最大轮数，返回错误
func (a *Agent) Run(ctx context.Context, messageCtx *agentctx.Context, input string) (string, error) {
	// 1. 注入SystemPrompt（首次对话时）
	messages, _ := a.ctxManager.GetMessages(messageCtx)
	if len(messages) == 0 && a.config.SystemPrompt != "" {
		systemMsg := &schema.Message{
			Role:    schema.System,
			Content: a.config.SystemPrompt,
		}
		if err := a.ctxManager.AddMessage(messageCtx, systemMsg); err != nil {
			return "", fmt.Errorf("failed to add system prompt: %w", err)
		}
	}

	// 2. 添加用户消息
	userMsg := &schema.Message{
		Role:    schema.User,
		Content: input,
	}
	if err := a.ctxManager.AddMessage(messageCtx, userMsg); err != nil {
		return "", fmt.Errorf("failed to add user message: %w", err)
	}

	// 3. ReAct 循环
	a.state.IsRunning = true
	defer func() { a.state.IsRunning = false }()

	for turn := 0; turn < a.config.MaxTurns; turn++ {
		a.state.CurrentTurn = turn + 1
		logger.DebugTag("REACT", "Turn %d/%d", turn+1, a.config.MaxTurns)

		// a. 获取所有消息
		messages, err := a.ctxManager.GetMessages(messageCtx)
		if err != nil {
			return "", fmt.Errorf("failed to get messages: %w", err)
		}
		logger.DebugTag("CTX", "Messages=%d", len(messages))
		for i, msg := range messages {
			contentPreview := logger.TruncateString(msg.Content, 40)
			logger.DebugTag("CTX", "  [%d] role=%-9s tools=%d content=%s",
				i, msg.Role, len(msg.ToolCalls), contentPreview)
		}

		// b. 调用 LLM 生成响应
		logger.DebugTag("LLM", "Calling Generate")
		resp, err := a.model.Generate(ctx, messages)
		if err != nil {
			logger.ErrorTag("LLM", "Generate failed: %v", err)
			return "", fmt.Errorf("LLM generation failed: %w", err)
		}
		logger.DebugTag("LLM", "Response received, tool_calls=%d", len(resp.ToolCalls))

		// c. 检查是否有工具调用
		if len(resp.ToolCalls) > 0 {
			logger.InfoTag("LLM", "Tool calls requested: %d", len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				logger.InfoTag("LLM", "  [%d] id=%s name=%s args=%s",
					i, tc.ID, tc.Function.Name, tc.Function.Arguments)
			}

			// d. 有工具调用 - 添加 assistant 消息
			assistantMsg := &schema.Message{
				Role:      schema.Assistant,
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			}
			if err := a.ctxManager.AddMessage(messageCtx, assistantMsg); err != nil {
				return "", fmt.Errorf("failed to add assistant message: %w", err)
			}

			// 执行工具
			if err := a.exeTools(ctx, messageCtx, resp.ToolCalls); err != nil {
				return "", fmt.Errorf("tool execution failed: %w", err)
			}

			// 继续循环
			continue
		}

		// e. 没有工具调用 - 返回响应
		assistantMsg := &schema.Message{
			Role:    schema.Assistant,
			Content: resp.Content,
		}
		if err := a.ctxManager.AddMessage(messageCtx, assistantMsg); err != nil {
			return "", fmt.Errorf("failed to add assistant message: %w", err)
		}

		return resp.Content, nil
	}

	// 4. 达到最大轮数
	return "", fmt.Errorf("reached max turns (%d) without final response", a.config.MaxTurns)
}

// TokenCallback 流式输出的回调函数类型
type TokenCallback func(token string)

// RunStream 运行 Agent 并流式输出响应
// 参数:
//   - ctx: Go 标准上下文
//   - messageCtx: 消息上下文
//   - input: 用户输入
//   - onToken: token 回调函数（每个 token 会调用一次）
//
// 返回: 完整响应内容和可能的错误
func (a *Agent) RunStream(ctx context.Context, messageCtx *agentctx.Context, input string, onToken TokenCallback) (string, error) {
	// 1. 注入SystemPrompt（首次对话时）
	messages, _ := a.ctxManager.GetMessages(messageCtx)
	if len(messages) == 0 && a.config.SystemPrompt != "" {
		systemMsg := &schema.Message{
			Role:    schema.System,
			Content: a.config.SystemPrompt,
		}
		if err := a.ctxManager.AddMessage(messageCtx, systemMsg); err != nil {
			return "", fmt.Errorf("failed to add system prompt: %w", err)
		}
	}

	// 2. 添加用户消息
	userMsg := &schema.Message{
		Role:    schema.User,
		Content: input,
	}
	if err := a.ctxManager.AddMessage(messageCtx, userMsg); err != nil {
		return "", fmt.Errorf("failed to add user message: %w", err)
	}

	// 2.5 检查是否需要压缩上下文
	// TODO:compress
	if a.ctxManager.ShouldCompress(messageCtx) {
		before, after, err := a.ctxManager.Compress(messageCtx)
		if err != nil {
			return "", fmt.Errorf("failed to compress context: %w", err)
		}
		logger.InfoTag("CTX", "Context compressed: %d -> %d messages", before, after)
		fmt.Printf("\n%s\n", logger.Yellow(fmt.Sprintf("[上下文压缩: %d -> %d 条消息]", before, after)))
	}

	// 3. ReAct 循环
	a.state.IsRunning = true
	defer func() { a.state.IsRunning = false }()

	for turn := 0; turn < a.config.MaxTurns; turn++ {
		a.state.CurrentTurn = turn + 1
		logger.DebugTag("REACT", "Turn %d/%d", turn+1, a.config.MaxTurns)

		// a. 获取所有消息
		messages, err := a.ctxManager.GetMessages(messageCtx)
		if err != nil {
			return "", fmt.Errorf("failed to get messages: %w", err)
		}
		logger.DebugTag("CTX", "Messages=%d", len(messages))
		for i, msg := range messages {
			contentPreview := logger.TruncateString(msg.Content, 40)
			logger.DebugTag("CTX", "  [%d] role=%-9s tools=%d content=%s",
				i, msg.Role, len(msg.ToolCalls), contentPreview)
		}

		// b. 调用 LLM 流式生成响应
		logger.DebugTag("LLM", "Calling Stream")
		reader, err := a.model.Stream(ctx, messages)
		if err != nil {
			logger.ErrorTag("LLM", "Stream failed: %v", err)
			return "", fmt.Errorf("LLM stream failed: %w", err)
		}

		logger.Debug("Stream started, reading chunks...")

		// 收集完整响应
		var fullContent string
		chunkCount := 0

		// 用于合并ToolCalls的列表（保持顺序）
		var toolCallsList []*schema.ToolCall
		// 用于快速查找最后一个工具调用的 map: id -> index
		toolCallsIndex := make(map[string]int)

		// 读取流式响应
		for {
			chunk, err := reader.Recv()
			if err == io.EOF {
				logger.DebugTag("STREAM", "EOF, chunks=%d", chunkCount)
				break
			}
			if err != nil {
				reader.Close()
				return "", fmt.Errorf("stream read failed: %w", err)
			}

			chunkCount++
			if chunkCount <= 3 {
				logger.DebugTag("STREAM", "Chunk#%d: len=%d role=%s tools=%d",
					chunkCount, len(chunk.Content), chunk.Role, len(chunk.ToolCalls))
			}

			// 详细记录包含ToolCalls的chunk
			if len(chunk.ToolCalls) > 0 {
				logger.DebugTag("STREAM", "Chunk#%d contains ToolCalls: %d", chunkCount, len(chunk.ToolCalls))
				for i, tc := range chunk.ToolCalls {
					logger.DebugTag("STREAM", "  [%d] id='%s' name='%s' args='%s'",
						i, tc.ID, tc.Function.Name, tc.Function.Arguments)

					// 如果有新的 ID，说明是新的工具调用
					if tc.ID != "" {
						// 检查是否已存在
						if idx, exists := toolCallsIndex[tc.ID]; exists {
							// 合并到已有的工具调用
							existing := toolCallsList[idx]
							if tc.Function.Name != "" && existing.Function.Name == "" {
								existing.Function.Name = tc.Function.Name
							}
							if tc.Function.Arguments != "" {
								existing.Function.Arguments += tc.Function.Arguments
							}
						} else {
							// 新建工具调用
							tcCopy := tc
							toolCallsList = append(toolCallsList, &tcCopy)
							toolCallsIndex[tc.ID] = len(toolCallsList) - 1
						}
					} else if tc.Function.Name != "" || tc.Function.Arguments != "" {
						// 没有 ID，但有 name 或 args，合并到最后一个工具调用
						if len(toolCallsList) > 0 {
							lastTC := toolCallsList[len(toolCallsList)-1]
							if tc.Function.Name != "" && lastTC.Function.Name == "" {
								lastTC.Function.Name = tc.Function.Name
							}
							if tc.Function.Arguments != "" {
								lastTC.Function.Arguments += tc.Function.Arguments
							}
						}
					}
				}
			}

			// 处理内容
			if chunk.Content != "" {
				fullContent += chunk.Content
				if onToken != nil {
					onToken(chunk.Content)
				}
			}
		}
		reader.Close()

		logger.DebugTag("STREAM", "Complete, total_len=%d", len(fullContent))

		// 构造最终消息：始终使用累积的 fullContent
		finalMessage := &schema.Message{
			Role:    schema.Assistant,
			Content: fullContent,
		}

		// 从列表中提取合并后的ToolCalls
		if len(toolCallsList) > 0 {
			// 记录合并后的ToolCalls
			logger.DebugTag("STREAM", "Merged ToolCalls: %d", len(toolCallsList))
			for i, tc := range toolCallsList {
				logger.DebugTag("STREAM", "  [%d] id='%s' name='%s' args='%s'",
					i, tc.ID, tc.Function.Name, tc.Function.Arguments)
			}

			// 过滤掉无效的 ToolCall（name 为空）
			validToolCalls := make([]schema.ToolCall, 0)
			for _, tc := range toolCallsList {
				if tc.Function.Name != "" {
					validToolCalls = append(validToolCalls, *tc)
				} else {
					logger.WarnTag("STREAM", "Filtered invalid ToolCall with empty name, id=%s", tc.ID)
				}
			}
			finalMessage.ToolCalls = validToolCalls
			logger.DebugTag("STREAM", "Valid ToolCalls=%d (filtered from %d)",
				len(validToolCalls), len(toolCallsList))
		}

		// c. 检查是否有工具调用
		if len(finalMessage.ToolCalls) > 0 {
			logger.InfoTag("LLM", "Tool calls requested: %d", len(finalMessage.ToolCalls))
			for i, tc := range finalMessage.ToolCalls {
				logger.InfoTag("LLM", "  [%d] id=%s name=%s args=%s",
					i, tc.ID, tc.Function.Name, tc.Function.Arguments)
			}

			// d. 有工具调用 - 添加 assistant 消息
			if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil {
				return "", fmt.Errorf("failed to add assistant message: %w", err)
			}

			// 执行工具
			if err := a.exeTools(ctx, messageCtx, finalMessage.ToolCalls); err != nil {
				return "", fmt.Errorf("tool execution failed: %w", err)
			}

			// 继续循环
			continue
		}

		// e. 没有工具调用 - 返回响应
		// 只有当内容不为空时才添加消息
		if fullContent != "" {
			if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil {
				return "", fmt.Errorf("failed to add assistant message: %w", err)
			}
		} else {
			logger.Warn("Skipping empty assistant message")
		}

		return fullContent, nil
	}

	// 4. 达到最大轮数
	return "", fmt.Errorf("reached max turns (%d) without final response", a.config.MaxTurns)
}

// exeTools 执行工具调用（支持并发）
// 参数:
//   - ctx: Go 标准上下文
//   - messageCtx: 消息上下文
//   - toolCalls: 工具调用列表
//
// 返回: 可能的错误
// 功能:
//  1. 分类工具：只读工具并发执行，写工具串行执行
//  2. 查找对应的工具
//  3. 执行工具
//  4. 将结果添加到 messageCtx
func (a *Agent) exeTools(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	logger.InfoTag("TOOL", "Executing %d tool(s)", len(toolCalls))

	// 定义只读工具列表
	readOnlyTools := map[string]bool{
		"read_file": true,
		"glob":      true,
		"grep":      true,
		"list_dir":  true,
		"task_get":  true,
		"task_list": true,
	}

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
		// 跳过无效的 ToolCall
		if tc.Function.Name == "" {
			logger.WarnTag("TOOL", "Skipping tool call with empty name, id=%s", tc.ID)
			continue
		}

		// 显示工具执行提示
		fmt.Printf("\n%s\n", logger.Cyan(fmt.Sprintf("[执行工具 %d/%d: %s]", idx+1, len(toolCalls), tc.Function.Name)))
		fmt.Printf("%s\n", logger.Gray(fmt.Sprintf("  参数: %s", tc.Function.Arguments)))
		logger.InfoTag("TOOL", "[%d/%d] name=%s id=%s", idx+1, len(toolCalls), tc.Function.Name, tc.ID)
		logger.InfoTag("TOOL", "  args: %s", tc.Function.Arguments)

		// 查找工具
		t := a.findTool(tc.Function.Name)
		if t == nil {
			logger.WarnTag("TOOL", "Not found: %s", tc.Function.Name)
			// 工具未找到，添加错误消息
			errMsg := schema.ToolMessage(
				fmt.Sprintf("tool not found: %s", tc.Function.Name),
				tc.ID,
			)
			if err := a.ctxManager.AddMessage(messageCtx, errMsg); err != nil {
				return fmt.Errorf("failed to add error message: %w", err)
			}
			continue
		}

		// 执行工具 - 优先尝试 EnhancedInvokableTool，然后尝试 InvokableTool
		var result string
		var execErr error

		logger.InfoTag("TOOL", "Invoking: %s", tc.Function.Name)

		// 尝试 EnhancedInvokableTool (返回 *schema.ToolResult)
		if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
			logger.DebugTag("TOOL", "Using EnhancedInvokableTool interface")
			toolArg := &schema.ToolArgument{
				Text: tc.Function.Arguments,
			}
			toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
			if err != nil {
				execErr = err
			} else {
				// 将 ToolResult 转换为字符串
				result = formatToolResult(toolResult)
			}
		} else if invokable, ok := t.(tool.InvokableTool); ok {
			// 尝试 InvokableTool (返回 string)
			logger.DebugTag("TOOL", "Using InvokableTool interface")
			result, execErr = invokable.InvokableRun(ctx, tc.Function.Arguments)
		} else {
			// 工具不支持任何可调用接口
			errMsg := schema.ToolMessage(
				fmt.Sprintf("tool %s is not invokable", tc.Function.Name),
				tc.ID,
			)
			if err := a.ctxManager.AddMessage(messageCtx, errMsg); err != nil {
				return fmt.Errorf("failed to add error message: %w", err)
			}
			continue
		}

		// 检查执行错误
		if execErr != nil {
			logger.ErrorTag("TOOL", "Failed: %s, err=%v", tc.Function.Name, execErr)
			errMsg := schema.ToolMessage(
				fmt.Sprintf("tool execution failed: %v", execErr),
				tc.ID,
			)
			if err := a.ctxManager.AddMessage(messageCtx, errMsg); err != nil {
				return fmt.Errorf("failed to add error message: %w", err)
			}
			continue
		}

		// 工具执行成功，添加结果
		logger.InfoTag("TOOL", "Success: %s", tc.Function.Name)
		logger.DebugTag("TOOL", "  result: %s", logger.TruncateString(result, 200))

		// 显示工具执行结果
		fmt.Printf("%s\n", logger.Green(fmt.Sprintf("  结果: %s", logger.TruncateString(result, 150))))

		resultMsg := schema.ToolMessage(result, tc.ID)
		if err := a.ctxManager.AddMessage(messageCtx, resultMsg); err != nil {
			return fmt.Errorf("failed to add tool result: %w", err)
		}
	}

	logger.InfoTag("TOOL", "All tools executed")
	return nil
}

// exeToolsConcurrent 并发执行只读工具
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
			fmt.Printf("\n%s\n", logger.Cyan(fmt.Sprintf("[执行工具 %d/%d: %s (并发)]", idx+1, len(toolCalls), tc.Function.Name)))
			fmt.Printf("%s\n", logger.Gray(fmt.Sprintf("  参数: %s", tc.Function.Arguments)))
			logger.InfoTag("TOOL", "[%d/%d] name=%s id=%s (concurrent)", idx+1, len(toolCalls), tc.Function.Name, tc.ID)

			// 查找工具
			t := a.findTool(tc.Function.Name)
			if t == nil {
				results <- toolResult{idx: idx, tc: tc, err: fmt.Errorf("tool not found: %s", tc.Function.Name)}
				return
			}

			// 执行工具
			logger.InfoTag("TOOL", "Invoking: %s", tc.Function.Name)

			var result string
			var execErr error

			if enhancedInvokable, ok := t.(tool.EnhancedInvokableTool); ok {
				toolArg := &schema.ToolArgument{Text: tc.Function.Arguments}
				toolResult, err := enhancedInvokable.InvokableRun(ctx, toolArg)
				if err != nil {
					execErr = err
				} else {
					result = formatToolResult(toolResult)
				}
			} else if invokable, ok := t.(tool.InvokableTool); ok {
				result, execErr = invokable.InvokableRun(ctx, tc.Function.Arguments)
			} else {
				execErr = fmt.Errorf("tool %s is not invokable", tc.Function.Name)
			}

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
		if res.err != nil {
			logger.ErrorTag("TOOL", "Failed: %s, err=%v", res.tc.Function.Name, res.err)
			errMsg := schema.ToolMessage(fmt.Sprintf("tool execution failed: %v", res.err), res.tc.ID)
			if err := a.ctxManager.AddMessage(messageCtx, errMsg); err != nil {
				return fmt.Errorf("failed to add error message: %w", err)
			}
			continue
		}

		logger.InfoTag("TOOL", "Success: %s", res.tc.Function.Name)
		fmt.Printf("%s\n", logger.Green(fmt.Sprintf("  结果: %s", logger.TruncateString(res.result, 150))))

		resultMsg := schema.ToolMessage(res.result, res.tc.ID)
		if err := a.ctxManager.AddMessage(messageCtx, resultMsg); err != nil {
			return fmt.Errorf("failed to add tool result: %w", err)
		}
	}

	return nil
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

// State 获取 Agent 状态
// 返回: Agent 状态
func (a *Agent) State() *State {
	return a.state
}
