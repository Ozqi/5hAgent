// Package agent 提供 AI Agent 的核心实现
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

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
	// token 预算（跨多次 Run/RunStream 调用持久化）
	tokenBudget *tokenBudget
}

// Config Agent 配置
type Config struct {
	Name            string // Agent 名称
	MaxTotalTokens  int    // 整场会话累计 token 上限
	RepeatToolLimit int    // 相同工具调用重复上限
	Debug           bool   // 是否启用调试
	SystemPrompt    string // 系统提示词
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
	if config.MaxTotalTokens == 0 {
		config.MaxTotalTokens = 1000000
	}
	if config.RepeatToolLimit == 0 {
		config.RepeatToolLimit = 5
	}

	// 初始化技能管理器
	skillMgr := skill.NewManager(".5hagent/skills")
	if err := skillMgr.LoadSkills(); err != nil {
		logger.DebugTag("SKILL", "Failed to load skills: %v", err)
	}

	return &Agent{
		model:        model,
		tools:        tools,
		toolMap:      buildToolMap(tools),
		config:       config,
		ctxManager:   agentctx.NewManager(),
		skillManager: skillMgr,
		tokenBudget:  newTokenBudget(config.MaxTotalTokens),
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
	if err := a.ensureConversationSetup(messageCtx); err != nil {
		return "", err
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
	repeatGuard := newToolRepeatGuard(a.config.RepeatToolLimit)

	for turn := 0; ; turn++ {
		a.state.CurrentTurn = turn + 1
		logger.DebugTag("REACT", "Turn %d", turn+1)

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
		if err := a.tokenBudget.Add(resp.ResponseMeta); err != nil {
			return "", err
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
			if err := repeatGuard.Check(resp.ToolCalls); err != nil {
				return "", err
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
}

// TokenCallback 流式输出的回调函数类型
type TokenCallback func(token string)

type tokenBudget struct {
	limit int
	used  int
	mu    sync.Mutex
}

func newTokenBudget(limit int) *tokenBudget {
	return &tokenBudget{limit: limit}
}

func (b *tokenBudget) Add(meta *schema.ResponseMeta) error {
	if b == nil || b.limit <= 0 || meta == nil || meta.Usage == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	used := meta.Usage.TotalTokens
	if used == 0 {
		used = meta.Usage.PromptTokens + meta.Usage.CompletionTokens
	}
	b.used += used
	if b.used > b.limit {
		return fmt.Errorf("max total tokens exceeded: %d > %d", b.used, b.limit)
	}
	return nil
}

func (b *tokenBudget) Usage() (used int, limit int) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.limit
}

type toolRepeatGuard struct {
	limit    int
	attempts map[string]int
}

func newToolRepeatGuard(limit int) *toolRepeatGuard {
	return &toolRepeatGuard{
		limit:    limit,
		attempts: make(map[string]int),
	}
}

func (g *toolRepeatGuard) Check(toolCalls []schema.ToolCall) error {
	if g == nil || g.limit <= 0 {
		return nil
	}
	for _, tc := range toolCalls {
		key := toolRepeatKey(tc)
		g.attempts[key]++
		if g.attempts[key] > g.limit {
			return fmt.Errorf("repeated tool call detected after %d attempts: %s", g.limit, tc.Function.Name)
		}
	}
	return nil
}

func toolRepeatKey(tc schema.ToolCall) string {
	return tc.Function.Name + ":" + normalizeToolArguments(tc.Function.Arguments)
}

func normalizeToolArguments(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return ""
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return trimmed
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return trimmed
	}
	return string(normalized)
}

func mergeResponseMeta(current *schema.ResponseMeta, incoming *schema.ResponseMeta) *schema.ResponseMeta {
	if incoming == nil {
		return current
	}
	if current == nil {
		return &schema.ResponseMeta{
			FinishReason: incoming.FinishReason,
			Usage:        cloneUsage(incoming.Usage),
		}
	}
	if incoming.FinishReason != "" {
		current.FinishReason = incoming.FinishReason
	}
	if incoming.Usage == nil {
		return current
	}
	if current.Usage == nil {
		current.Usage = cloneUsage(incoming.Usage)
		return current
	}
	if incoming.Usage.PromptTokens > current.Usage.PromptTokens {
		current.Usage.PromptTokens = incoming.Usage.PromptTokens
		current.Usage.PromptTokenDetails = incoming.Usage.PromptTokenDetails
	}
	if incoming.Usage.CompletionTokens > current.Usage.CompletionTokens {
		current.Usage.CompletionTokens = incoming.Usage.CompletionTokens
	}
	if incoming.Usage.TotalTokens > current.Usage.TotalTokens {
		current.Usage.TotalTokens = incoming.Usage.TotalTokens
	}
	if current.Usage.TotalTokens == 0 {
		current.Usage.TotalTokens = current.Usage.PromptTokens + current.Usage.CompletionTokens
	}
	return current
}

func cloneUsage(usage *schema.TokenUsage) *schema.TokenUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
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
	if err := a.ensureConversationSetup(messageCtx); err != nil {
		return "", err
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
	}

	// 3. ReAct 循环
	a.state.IsRunning = true
	defer func() { a.state.IsRunning = false }()
	repeatGuard := newToolRepeatGuard(a.config.RepeatToolLimit)

	for turn := 0; ; turn++ {
		a.state.CurrentTurn = turn + 1
		logger.DebugTag("REACT", "Turn %d", turn+1)

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
		var responseMeta *schema.ResponseMeta

		toolQueue := make(chan streamToolRequest, 8)
		toolResultCh := make(chan streamToolResult, 8)
		queuedCalls := make([]schema.ToolCall, 0)

		go func() {
			for req := range toolQueue {
				result, execErr := a.executeToolCall(ctx, req.tc, req.idx, req.idx+1, false)
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
			responseMeta = mergeResponseMeta(responseMeta, chunk.ResponseMeta)
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
					if err := repeatGuard.Check([]schema.ToolCall{tc}); err != nil {
						reader.Close()
						close(toolQueue)
						for range toolResultCh {
						}
						return "", err
					}
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
		if err := a.tokenBudget.Add(responseMeta); err != nil {
			for range toolResultCh {
			}
			return "", err
		}

		toolCalls := collector.RunnableCalls()
		toolResults := make([]streamToolResult, len(queuedCalls))
		for res := range toolResultCh {
			toolResults[res.idx] = res
		}

		finalMessage := &schema.Message{
			Role:         schema.Assistant,
			Content:      content,
			ToolCalls:    toolCalls,
			ResponseMeta: responseMeta,
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

func (a *Agent) GetModel() model.ToolCallingChatModel {
	if a == nil {
		return nil
	}
	return a.model
}

// SetTools 设置工具列表
func (a *Agent) SetTools(tools []tool.BaseTool) {
	a.tools = tools
	a.toolMap = buildToolMap(tools)
}

func (a *Agent) Name() string {
	if a == nil || a.config == nil {
		return ""
	}
	return a.config.Name
}

func (a *Agent) TokenUsage() (used int, limit int) {
	if a == nil || a.tokenBudget == nil {
		return 0, 0
	}
	return a.tokenBudget.Usage()
}

func buildToolMap(tools []tool.BaseTool) map[string]tool.BaseTool {
	toolMap := make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		toolMap[info.Name] = t
	}
	return toolMap
}

func (a *Agent) ensureConversationSetup(messageCtx *agentctx.Context) error {
	messages, _ := a.ctxManager.GetMessages(messageCtx)
	if len(messages) > 0 {
		return nil
	}

	if a.config.SystemPrompt != "" {
		systemMsg := &schema.Message{
			Role:    schema.System,
			Content: a.config.SystemPrompt,
		}
		if err := a.ctxManager.AddMessage(messageCtx, systemMsg); err != nil {
			return fmt.Errorf("failed to add system prompt: %w", err)
		}
	}

	if err := a.injectSkills(messageCtx); err != nil {
		return fmt.Errorf("failed to inject skills: %w", err)
	}

	return nil
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
