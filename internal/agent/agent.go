// Package agent 提供 AI Agent 的核心实现
package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/skill"
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
	// 技能管理器
	skillManager *skill.Manager // 技能注入管理
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
//  4. 初始化技能管理器
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

	// 初始化技能管理器
	skillMgr := skill.NewManager(".5hagent/skills")
	if err := skillMgr.LoadSkills(); err != nil {
		logger.DebugTag("SKILL", "Failed to load skills: %v", err)
	}

	return &Agent{
		model:        model,
		tools:        tools,
		toolMap:      toolMap,
		config:       config,
		ctxManager:   agentctx.NewManager(),
		skillManager: skillMgr,
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
	// 1. 注入SystemPrompt和Skills（首次对话时）
	messages, _ := a.ctxManager.GetMessages(messageCtx)
	if len(messages) == 0 {
		// 添加 system prompt
		if a.config.SystemPrompt != "" {
			systemMsg := &schema.Message{
				Role:    schema.System,
				Content: a.config.SystemPrompt,
			}
			if err := a.ctxManager.AddMessage(messageCtx, systemMsg); err != nil {
				return "", fmt.Errorf("failed to add system prompt: %w", err)
			}
		}

		// 注入启用的技能作为独立消息
		if err := a.injectSkills(messageCtx); err != nil {
			return "", fmt.Errorf("failed to inject skills: %w", err)
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
			logger.DebugTag("LLM", "Tool calls requested: %d", len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				logger.DebugTag("LLM", "  [%d] id=%s name=%s args=%s",
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
	// 1. 注入SystemPrompt和Skills（首次对话时）
	messages, _ := a.ctxManager.GetMessages(messageCtx)
	if len(messages) == 0 {
		// 添加 system prompt
		if a.config.SystemPrompt != "" {
			systemMsg := &schema.Message{
				Role:    schema.System,
				Content: a.config.SystemPrompt,
			}
			if err := a.ctxManager.AddMessage(messageCtx, systemMsg); err != nil {
				return "", fmt.Errorf("failed to add system prompt: %w", err)
			}
		}

		// 注入启用的技能作为独立消息
		if err := a.injectSkills(messageCtx); err != nil {
			return "", fmt.Errorf("failed to inject skills: %w", err)
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
	//  TODO: 这里只有一个按照消息条数压缩。
	if a.ctxManager.ShouldCompress(messageCtx) {
		before, after, err := a.ctxManager.Compress(messageCtx)
		if err != nil {
			return "", fmt.Errorf("failed to compress context: %w", err)
		}
		logger.DebugTag("CTX", "Context compressed: %d -> %d messages", before, after)
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

		var toolCallsList []*schema.ToolCall   // 用于合并ToolCalls的列表（保持顺序）
		toolCallsIndex := make(map[string]int) // 用于快速查找最后一个工具调用的 map: id -> index
		executedTools := make(map[string]bool) // 记录已执行的工具（避免重复执行）

		for { // 读取流式响应
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
			// if chunkCount <= 10 {
			// 	logger.DebugTag("STREAM", "Chunk#%d: len=%d role=%s tools=%d",
			// 		chunkCount, len(chunk.Content), chunk.Role, len(chunk.ToolCalls))
			// }

			// *处理包含ToolCalls的chunk
			if len(chunk.ToolCalls) > 0 {
				logger.DebugTag("STREAM", "Chunk#%d contains ToolCalls: %d", chunkCount, len(chunk.ToolCalls))
				for i, tc := range chunk.ToolCalls {
					logger.DebugTag("STREAM", "  [%d] id='%s' name='%s' args='%s'",
						i, tc.ID, tc.Function.Name, tc.Function.Arguments)

					// 如果有新的 ID，说明是新的工具调用
					if tc.ID != "" {
						if idx, exists := toolCallsIndex[tc.ID]; exists { // 检查是否已存在
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

			// 边输出边执行：检查是否有完整的工具调用可以执行
			for _, tc := range toolCallsList {
				// 检查工具调用是否完整且未执行
				if tc.ID != "" && tc.Function.Name != "" && !executedTools[tc.ID] {
					// 尝试解析参数，判断是否完整
					if isValidJSON(tc.Function.Arguments) {
						logger.DebugTag("STREAM", "Tool ready for execution: id=%s name=%s", tc.ID, tc.Function.Name)

						executedTools[tc.ID] = true // 标记为已执行

						// 立即执行工具（在 goroutine 中异步执行，避免阻塞流式输出）
						go func(toolCall *schema.ToolCall) {
							a.executeToolStreaming(ctx, messageCtx, toolCall)
						}(tc)
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
			logger.DebugTag("LLM", "Tool calls requested: %d", len(finalMessage.ToolCalls))
			for i, tc := range finalMessage.ToolCalls {
				logger.DebugTag("LLM", "  [%d] id=%s name=%s args=%s",
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
// GetSkillManager 获取技能管理器
func (a *Agent) GetSkillManager() *skill.Manager {
	return a.skillManager
}

// SetModel 设置模型
func (a *Agent) SetModel(model model.ToolCallingChatModel) {
	a.model = model
}

// SetTools 设置工具列表
func (a *Agent) SetTools(tools []tool.BaseTool) {
	a.tools = tools
	// 重建工具映射表
	a.toolMap = make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		a.toolMap[info.Name] = t
	}
}

// injectSkills 将启用的技能作为独立消息注入到上下文
func (a *Agent) injectSkills(messageCtx *agentctx.Context) error {
	skills := a.skillManager.ListSkills()
	for _, skill := range skills {
		if skill.Enabled {
			skillMsg := &schema.Message{
				Role:    schema.System,
				Content: fmt.Sprintf("# Skill: %s\n\n%s", skill.Name, skill.Content),
			}
			if err := a.ctxManager.AddMessage(messageCtx, skillMsg); err != nil {
				return fmt.Errorf("failed to add skill %s: %w", skill.Name, err)
			}
		}
	}
	return nil
}
