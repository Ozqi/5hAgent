// Package agent 提供 AI Agent 的核心实现
package agent

import (
	"context"
	"fmt"
	"io"

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

		// 用于合并ToolCalls的map: id -> ToolCall
		toolCallsMap := make(map[string]*schema.ToolCall)

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
				logger.InfoTag("STREAM", "Chunk#%d contains ToolCalls: %d", chunkCount, len(chunk.ToolCalls))
				for i, tc := range chunk.ToolCalls {
					logger.InfoTag("STREAM", "  [%d] id='%s' name='%s' args='%s'",
						i, tc.ID, tc.Function.Name, tc.Function.Arguments)

					// 合并ToolCall信息
					// 如果有ID，使用ID作为key；否则使用索引
					key := tc.ID
					if key == "" {
						key = fmt.Sprintf("_index_%d", i)
					}

					if existing, ok := toolCallsMap[key]; ok {
						// 合并：补充空字段
						if tc.ID != "" && existing.ID == "" {
							existing.ID = tc.ID
						}
						if tc.Function.Name != "" && existing.Function.Name == "" {
							existing.Function.Name = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							existing.Function.Arguments += tc.Function.Arguments
						}
					} else {
						// 新建
						tcCopy := tc
						toolCallsMap[key] = &tcCopy
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

		// 从map中提取合并后的ToolCalls
		if len(toolCallsMap) > 0 {
			mergedToolCalls := make([]schema.ToolCall, 0, len(toolCallsMap))
			for _, tc := range toolCallsMap {
				mergedToolCalls = append(mergedToolCalls, *tc)
			}

			// 记录合并后的ToolCalls
			logger.InfoTag("STREAM", "Merged ToolCalls: %d", len(mergedToolCalls))
			for i, tc := range mergedToolCalls {
				logger.InfoTag("STREAM", "  [%d] id='%s' name='%s' args='%s'",
					i, tc.ID, tc.Function.Name, tc.Function.Arguments)
			}

			// 过滤掉无效的 ToolCall（name 为空）
			validToolCalls := make([]schema.ToolCall, 0)
			for _, tc := range mergedToolCalls {
				if tc.Function.Name != "" {
					validToolCalls = append(validToolCalls, tc)
				} else {
					logger.WarnTag("STREAM", "Filtered invalid ToolCall with empty name, id=%s", tc.ID)
				}
			}
			finalMessage.ToolCalls = validToolCalls
			logger.DebugTag("STREAM", "Valid ToolCalls=%d (filtered from %d)",
				len(validToolCalls), len(mergedToolCalls))
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

// exeTools 执行工具调用
// 参数:
//   - ctx: Go 标准上下文
//   - messageCtx: 消息上下文
//   - toolCalls: 工具调用列表
//
// 返回: 可能的错误
// 功能:
//  1. 遍历工具调用列表
//  2. 查找对应的工具
//  3. 执行工具
//  4. 将结果添加到 messageCtx
func (a *Agent) exeTools(ctx context.Context, messageCtx *agentctx.Context, toolCalls []schema.ToolCall) error {
	logger.InfoTag("TOOL", "Executing %d tool(s)", len(toolCalls))

	for idx, tc := range toolCalls {
		// 跳过无效的 ToolCall
		if tc.Function.Name == "" {
			logger.WarnTag("TOOL", "Skipping tool call with empty name, id=%s", tc.ID)
			continue
		}

		// 显示工具执行提示
		fmt.Printf("\n%s\n", logger.Cyan(fmt.Sprintf("[执行工具 %d/%d: %s]", idx+1, len(toolCalls), tc.Function.Name)))
		logger.InfoTag("TOOL", "[%d/%d] name=%s id=%s", idx+1, len(toolCalls), tc.Function.Name, tc.ID)
		logger.DebugTag("TOOL", "  args: %s", tc.Function.Arguments)

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

		// 类型断言为 InvokableTool
		invokable, ok := t.(tool.InvokableTool)
		if !ok {
			errMsg := schema.ToolMessage(
				fmt.Sprintf("tool %s is not invokable", tc.Function.Name),
				tc.ID,
			)
			if err := a.ctxManager.AddMessage(messageCtx, errMsg); err != nil {
				return fmt.Errorf("failed to add error message: %w", err)
			}
			continue
		}

		// 执行工具
		logger.InfoTag("TOOL", "Invoking: %s", tc.Function.Name)
		result, err := invokable.InvokableRun(ctx, tc.Function.Arguments)
		if err != nil {
			// 工具执行失败
			logger.ErrorTag("TOOL", "Failed: %s, err=%v", tc.Function.Name, err)
			errMsg := schema.ToolMessage(
				fmt.Sprintf("tool execution failed: %v", err),
				tc.ID,
			)
			if err := a.ctxManager.AddMessage(messageCtx, errMsg); err != nil {
				return fmt.Errorf("failed to add error message: %w", err)
			}
			continue
		}

		// 工具执行成功，添加结果
		logger.InfoTag("TOOL", "Success: %s", tc.Function.Name)
		logger.DebugTag("TOOL", "  result: %s", result)
		resultMsg := schema.ToolMessage(result, tc.ID)
		if err := a.ctxManager.AddMessage(messageCtx, resultMsg); err != nil {
			return fmt.Errorf("failed to add tool result: %w", err)
		}
	}

	logger.InfoTag("TOOL", "All tools executed")
	return nil
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
