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

type streamToolState struct {
	call       schema.ToolCall
	dispatched bool
}

type streamToolCollector struct {
	states []*streamToolState
	byID   map[string]int
}

func newStreamToolCollector() *streamToolCollector {
	return &streamToolCollector{
		byID: make(map[string]int),
	}
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

func (a *Agent) executeToolCall(ctx context.Context, tc schema.ToolCall, idx, total int) (string, error) {
	logger.PrintToolCall(tc.Function.Name, tc.Function.Arguments, false)
	logger.DebugTag("TOOL", "[%d/%d] name=%s id=%s", idx+1, total, tc.Function.Name, tc.ID)
	logger.DebugTag("TOOL", "  args: %s", tc.Function.Arguments)

	t := a.findTool(tc.Function.Name)
	if t == nil {
		logger.WarnTag("TOOL", "Not found: %s", tc.Function.Name)
		return "", fmt.Errorf("tool not found: %s", tc.Function.Name)
	}

	return a.invokeTool(ctx, t, tc)
}

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
	if a.ctxManager.ShouldCompress(messageCtx) {
		before, after, err := a.ctxManager.LMCompress(ctx, messageCtx, a.model, "prompt")
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

		var fullContent strings.Builder
		chunkCount := 0
		collector := newStreamToolCollector()

		toolQueue := make(chan streamToolRequest, 8)
		toolResultCh := make(chan streamToolResult, 8)
		queuedCalls := make([]schema.ToolCall, 0)

		go func() {
			for req := range toolQueue {
				result, execErr := a.executeToolCall(ctx, req.tc, req.idx, req.idx+1)
				toolResultCh <- streamToolResult{idx: req.idx, tc: req.tc, result: result, err: execErr}
			}
			close(toolResultCh)
		}()

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
				}

				for _, tc := range collector.Add(chunk.ToolCalls) {
					idx := len(queuedCalls)
					queuedCalls = append(queuedCalls, tc)
					toolQueue <- streamToolRequest{idx: idx, tc: tc}
				}
			}

			// 处理内容
			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
				if onToken != nil {
					onToken(chunk.Content)
				}
			}
		}
		reader.Close()
		close(toolQueue)

		content := fullContent.String()
		logger.DebugTag("STREAM", "Complete, total_len=%d", len(content))

		toolCalls := collector.RunnableCalls()
		toolResults := make([]streamToolResult, len(queuedCalls))
		for res := range toolResultCh {
			toolResults[res.idx] = res
		}

		finalMessage := &schema.Message{
			Role:      schema.Assistant,
			Content:   content,
			ToolCalls: toolCalls,
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

			for _, res := range toolResults {
				if err := a.addToolResultToContext(messageCtx, res.tc, res.result, res.err); err != nil {
					return "", fmt.Errorf("tool execution failed: %w", err)
				}
			}

			// 继续循环
			continue
		}

		// e. 没有工具调用 - 返回响应
		// 只有当内容不为空时才添加消息
		if content != "" {
			if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil {
				return "", fmt.Errorf("failed to add assistant message: %w", err)
			}
		} else {
			logger.Warn("Skipping empty assistant message")
		}

		return content, nil
	}

	// 4. 达到最大轮数
	return "", fmt.Errorf("reached max turns (%d) without final response", a.config.MaxTurns)
}

// exeTools 执行工具调用（支持并发）
// 参数:
//   - ctx: Go 标准上下文
//   - messageCtx: 消息上下文
//
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
