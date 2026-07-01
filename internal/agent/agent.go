// agent.go - Agent 核心实现
// 功能：ReAct 循环、LLM 调用、工具执行协调、上下文管理
// 主要类型：Agent, Config, State, tokenBudget, toolRepeatGuard
// 导出函数：NewAgent, RunStream, GetSkillManager, SetModel, SetTools, SetToolEventSink, Name, TokenUsage
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/utils"
)

// Agent AI Agent 核心结构体
// 负责协调 LLM、工具、上下文管理器
type Agent struct {
	// 核心组件
	model   model.ToolCallingChatModel // LLM 模型
	tools   []tool.BaseTool            // 工具列表
	toolMap map[string]tool.BaseTool   // 工具名称映射表

	// 配置
	config *Config // Agent 配置
	// 状态
	state *State // Agent 运行状态
	// 上下文管理器
	ctxManager *agentctx.Manager // 消息历史管理
	// 技能管理器
	skillManager *skill.Manager // 技能注入管理
	// token 预算
	tokenBudget *utils.TokenBudget
	// 回调处理器
	callbacks *AgentCallbacks
	// 工具事件 sink
	toolEventSink func(logger.ToolEvent)
}

// Config Agent 配置
type Config struct {
	Name                string // Agent 名称
	MaxTotalTokens      int    // 整场会话累计 token 上限
	RepeatToolLimit     int    // 相同工具调用重复上限
	Debug               bool   // 是否启用调试
	ContextAutoCompress bool   // 是否自动触发上下文压缩
	DisableStream       bool   // 是否禁用流式模型调用；部分兼容供应商需要关闭
	SystemPrompt        string // 系统提示词
	ProjectDataDir      string // 项目 .5hagent 数据目录；为空时使用当前工作目录
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

	configDir, err := utils.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	projectDataDir := config.ProjectDataDir
	if projectDataDir == "" {
		var err error
		projectDataDir, err = utils.GetProjectDataDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get project data directory: %w", err)
		}
	}
	skillMgr := skill.NewManagerFromDirs(
		skill.Source{Scope: "global", Dir: filepath.Join(configDir, "skills")},
		skill.Source{Scope: "project", Dir: filepath.Join(projectDataDir, "skills")},
	)
	if err := skillMgr.LoadSkills(); err != nil {
		logger.DebugTag("SKILL", "Failed to load skills: %v", err)
	}

	toolMap := make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		toolMap[info.Name] = t
	}

	return &Agent{
		model:        model,
		tools:        tools,
		toolMap:      toolMap,
		config:       config,
		ctxManager:   agentctx.NewManager(),
		skillManager: skillMgr,
		tokenBudget:  utils.NewTokenBudget(config.MaxTotalTokens),
		state: &State{
			CurrentTurn: 0,
			IsRunning:   false,
		},
		callbacks: NewAgentCallbacks(config.Debug),
	}, nil
}

// SetCtxManager 设置上下文管理器（用于 session 持久化）
func (a *Agent) SetCtxManager(manager *agentctx.Manager) {
	a.ctxManager = manager
}

// SetToolEventSink 设置当前 Agent 的工具事件接收器，并返回旧接收器。
// 参数：sink 接收 tool call/result/error/status 事件；nil 表示回退到 logger 默认输出。
// 调用层级：TUI/headless/runtime -> SetToolEventSink -> exeToolCall/addToolResult。
// 步骤：只替换当前 Agent 实例字段，不改包级 logger sink。
func (a *Agent) SetToolEventSink(sink func(logger.ToolEvent)) func(logger.ToolEvent) {
	prev := a.toolEventSink
	a.toolEventSink = sink
	return prev
}

// GetCtxManager 获取上下文管理器
func (a *Agent) GetCtxManager() *agentctx.Manager {
	return a.ctxManager
}

// TokenCallback 流式输出的回调函数类型
type TokenCallback func(token string)

// toolRepeatGuard 工具重复调用防护结构
// 限制同一工具（含相同参数）被重复调用的次数，防止死循环
type toolRepeatGuard struct {
	limit    int
	attempts map[string]int
}

// newToolRepeatGuard 创建工具重复调用防护实例
// 参数:
//   - limit: 单工具（含相同参数）最大重复次数
//
// 返回: toolRepeatGuard 实例
func newToolRepeatGuard(limit int) *toolRepeatGuard {
	return &toolRepeatGuard{
		limit:    limit,
		attempts: make(map[string]int),
	}
}

// Check 检查工具调用是否超限
// 参数:
//   - toolCalls: 待检查的工具调用列表
//
// 返回: 超限返回错误，否则返回 nil
func (g *toolRepeatGuard) Check(toolCalls []schema.ToolCall) error {
	if g == nil || g.limit <= 0 {
		return nil
	}
	for _, tc := range toolCalls {
		args := tc.Function.Arguments
		trimmed := strings.TrimSpace(args)
		key := tc.Function.Name + ":"
		if trimmed != "" {
			var decoded interface{}
			if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
				key += trimmed
			} else if normalized, err := json.Marshal(decoded); err == nil {
				key += string(normalized)
			} else {
				key += trimmed
			}
		}
		g.attempts[key]++
		if g.attempts[key] > g.limit {
			return fmt.Errorf("repeated tool call detected after %d attempts: %s", g.limit, tc.Function.Name)
		}
	}
	return nil
}

// mergeMeta 合并 LLM 流式响应元数据
// 参数:
//   - current: 当前累计的响应元数据（可能被 nil）
//   - incoming: 新到来的响应元数据
//
// 返回: 合并后的元数据（优先保留较大的 token 计数）
func mergeMeta(current *schema.ResponseMeta, incoming *schema.ResponseMeta) *schema.ResponseMeta {
	if incoming == nil {
		return current
	}
	if current == nil {
		cloned := *incoming
		return &cloned
	}
	if incoming.FinishReason != "" {
		current.FinishReason = incoming.FinishReason
	}
	if incoming.Usage == nil {
		return current
	}
	if current.Usage == nil {
		cloned := *incoming.Usage
		current.Usage = &cloned
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

// RunStream 运行 Agent 并流式输出响应
// 参数:
//   - ctx: Go 标准上下文
//   - messageCtx: 消息上下文
//   - input: 用户输入
//   - onToken: 正文 token 回调函数（每个 token 会调用一次）
//   - onReasoning: 可选 thinking/reasoning token 回调函数
//
// 返回: 完整响应内容和可能的错误
func (a *Agent) RunStream(ctx context.Context, messageCtx *agentctx.Context, input string, onToken TokenCallback, onReasoning ...TokenCallback) (string, error) {
	return a.RunStreamWithOptions(ctx, messageCtx, input, onToken, nil, onReasoning...)
}

// RunStreamWithOptions 运行 Agent，并为本次模型调用追加临时 model options。
// 参数：opts 只影响当前 RunStream 调用，不改变 Agent 持有的模型和工具列表。
// 调用层级：runtime.RunProcess/RunTaskOnce -> RunStreamWithOptions -> model.Stream。
// 步骤：沿用 ReAct 循环；每轮 Stream 传入 opts；工具执行路径保持不变。
func (a *Agent) RunStreamWithOptions(ctx context.Context, messageCtx *agentctx.Context, input string, onToken TokenCallback, opts []model.Option, onReasoning ...TokenCallback) (string, error) {
	var reasoningCallback TokenCallback
	if len(onReasoning) > 0 {
		reasoningCallback = onReasoning[0]
	}
	opts = append([]model.Option{}, opts...)
	opts = append(opts, a.forcedToolOptions(input)...)

	// 1. 注入SystemPrompt和Skills（首次对话时）
	if err := a.ensureConversationSetup(messageCtx); err != nil {
		return "", err
	}

	// 2. 添加用户消息
	ctx = agentctx.WithToolRuntime(ctx, a.ctxManager, messageCtx)
	userMsg := &schema.Message{
		Role:    schema.User,
		Content: input,
	}
	if err := a.ctxManager.AddMessage(messageCtx, userMsg); err != nil {
		return "", fmt.Errorf("failed to add user message: %w", err)
	}

	// 2.5 检查是否需要压缩上下文
	if a.config.ContextAutoCompress && a.ctxManager.ShouldCompress(messageCtx) {
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
		if a.config.Debug {
			logger.DebugTag("REACT", "Turn %d", turn+1)
		}

		// a. 获取所有消息
		messages, err := a.ctxManager.GetMessages(messageCtx)
		if err != nil {
			return "", fmt.Errorf("failed to get messages: %w", err)
		}
		if a.config.Debug {
			logger.DebugTag("CTX", "Messages=%d", len(messages))
		}

		// b. 调用 LLM 生成响应（使用 Callback）
		cb := a.callbacks
		cb.OnModelStart(ctx, nil, &model.CallbackInput{Messages: messages})
		if a.config.DisableStream {
			msg, err := a.model.Generate(ctx, messages, opts...)
			if err != nil {
				cb.OnModelError(ctx, nil, err)
				return "", fmt.Errorf("LLM generate failed: %w", err)
			}
			if msg == nil {
				err := fmt.Errorf("LLM generate returned nil message")
				cb.OnModelError(ctx, nil, err)
				return "", err
			}

			var tokenUsage *model.TokenUsage
			if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
				tokenUsage = &model.TokenUsage{
					PromptTokens:     msg.ResponseMeta.Usage.PromptTokens,
					CompletionTokens: msg.ResponseMeta.Usage.CompletionTokens,
					TotalTokens:      msg.ResponseMeta.Usage.TotalTokens,
				}
			}
			cb.OnModelEnd(ctx, nil, &model.CallbackOutput{Message: msg, TokenUsage: tokenUsage})

			if msg.ReasoningContent != "" && reasoningCallback != nil {
				reasoningCallback(msg.ReasoningContent)
			}
			if msg.Content != "" && onToken != nil {
				onToken(msg.Content)
			}

			toolCalls := make([]schema.ToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "" {
					logger.WarnTag("TOOL", "Skipping incomplete tool call: id=%s name=%s", tc.ID, tc.Function.Name)
					continue
				}
				toolCalls = append(toolCalls, tc)
			}

			finalMessage := &schema.Message{
				Role:             schema.Assistant,
				Content:          msg.Content,
				ReasoningContent: msg.ReasoningContent,
				ToolCalls:        toolCalls,
				ResponseMeta:     msg.ResponseMeta,
				Extra:            msg.Extra,
			}

			if len(finalMessage.ToolCalls) > 0 {
				cb.LogToolCalls(finalMessage.ToolCalls)
				if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil {
					return "", fmt.Errorf("failed to add assistant message: %w", err)
				}
				for idx, tc := range finalMessage.ToolCalls {
					result, execErr := a.exeToolCall(ctx, tc, idx, len(finalMessage.ToolCalls), false)
					if execErr == nil {
						if err := repeatGuard.Check([]schema.ToolCall{tc}); err != nil {
							logger.WarnTag("TOOL", "%v", err)
						}
					}
					if err := a.addToolResult(messageCtx, tc, result, execErr); err != nil {
						return "", fmt.Errorf("tool execution failed: %w", err)
					}
				}
				continue
			}

			if msg.Content != "" {
				if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil {
					return "", fmt.Errorf("failed to add assistant message: %w", err)
				}
			} else {
				logger.Debug("Skipping empty assistant message")
			}
			return msg.Content, nil
		}

		streamCtx, streamCancel := context.WithCancel(ctx)
		reader, err := a.model.Stream(streamCtx, messages, opts...)
		if err != nil {
			streamCancel()
			cb.OnModelError(ctx, nil, err)
			return "", fmt.Errorf("LLM stream failed: %w", err)
		}

		var fullContent strings.Builder
		var fullReasoning strings.Builder
		var fullExtra map[string]any
		chunkCount := 0
		collector := newToolCollector()
		var responseMeta *schema.ResponseMeta

		toolQueue := make(chan toolRequest, 8)
		toolResultCh := make(chan execResult, 8)
		queuedCalls := make([]schema.ToolCall, 0)

		go func() {
			for req := range toolQueue {
				result, execErr := a.exeToolCall(ctx, req.tc, req.idx, req.idx+1, false)
				// 只对成功的调用计数重复，失败的不计入
				if execErr == nil {
					if err := repeatGuard.Check([]schema.ToolCall{req.tc}); err != nil {
						logger.WarnTag("TOOL", "%v", err)
					}
				}
				toolResultCh <- execResult{idx: req.idx, tc: req.tc, result: result, err: execErr}
			}
			close(toolResultCh)
		}()

		// 读取流式响应
		for {
			type recvResult struct {
				chunk *schema.Message
				err   error
			}
			recvCh := make(chan recvResult, 1)
			go func() {
				chunk, err := reader.Recv()
				recvCh <- recvResult{chunk: chunk, err: err}
			}()

			var chunk *schema.Message
			var err error
			select {
			case res := <-recvCh:
				chunk, err = res.chunk, res.err
			case <-time.After(90 * time.Second):
				streamCancel()
				reader.Close()
				cb.OnModelError(ctx, nil, context.DeadlineExceeded)
				return "", fmt.Errorf("LLM stream idle timeout: no text or tool call chunk received for 90s")
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				reader.Close()
				streamCancel()
				cb.OnModelError(ctx, nil, err)
				return "", fmt.Errorf("stream read failed: %w", err)
			}

			chunkCount++
			responseMeta = mergeMeta(responseMeta, chunk.ResponseMeta)
			fullExtra = mergeMessageExtra(fullExtra, chunk.Extra)

			// 处理 ToolCalls
			if len(chunk.ToolCalls) > 0 {
				cb.LogChunk(chunk, chunkCount)

				for _, tc := range collector.Add(chunk.ToolCalls) {
					idx := len(queuedCalls)
					queuedCalls = append(queuedCalls, tc)
					toolQueue <- toolRequest{idx: idx, tc: tc}
				}
			}

			// 处理内容
			if chunk.ReasoningContent != "" {
				fullReasoning.WriteString(chunk.ReasoningContent)
				if reasoningCallback != nil {
					reasoningCallback(chunk.ReasoningContent)
				}
			}

			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
				if onToken != nil {
					onToken(chunk.Content)
				}
			}
		}
		reader.Close()
		streamCancel()

		for _, tc := range collector.PendingRunnableCalls() {
			idx := len(queuedCalls)
			queuedCalls = append(queuedCalls, tc)
			toolQueue <- toolRequest{idx: idx, tc: tc}
		}
		close(toolQueue)

		content := fullContent.String()
		reasoningContent := fullReasoning.String()

		// 转换 token usage 类型
		var tokenUsage *model.TokenUsage
		if responseMeta != nil && responseMeta.Usage != nil {
			tokenUsage = &model.TokenUsage{
				PromptTokens:     responseMeta.Usage.PromptTokens,
				CompletionTokens: responseMeta.Usage.CompletionTokens,
				TotalTokens:      responseMeta.Usage.TotalTokens,
			}
		}
		cb.OnModelEnd(ctx, nil, &model.CallbackOutput{
			Message:    &schema.Message{Content: content, ResponseMeta: responseMeta},
			TokenUsage: tokenUsage,
		})

		toolCalls := make([]schema.ToolCall, len(queuedCalls))
		copy(toolCalls, queuedCalls)

		// 过滤掉没有工具名的调用（流式输出中不完整的调用），避免发给 LLM 造成格式错误
		var validCalls []schema.ToolCall
		for _, tc := range toolCalls {
			if tc.Function.Name == "" {
				logger.WarnTag("TOOL", "Skipping incomplete tool call: id=%s name=%s", tc.ID, tc.Function.Name)
				continue
			}
			validCalls = append(validCalls, tc)
		}
		toolCalls = validCalls

		// 收集已执行的结果（按 queuedCalls 顺序）
		toolResults := make([]execResult, len(queuedCalls))
		for res := range toolResultCh {
			toolResults[res.idx] = res
		}

		finalMessage := &schema.Message{
			Role:             schema.Assistant,
			Content:          content,
			ReasoningContent: reasoningContent,
			ToolCalls:        toolCalls,
			ResponseMeta:     responseMeta,
			Extra:            fullExtra,
		}

		// c. 检查是否有工具调用
		if len(finalMessage.ToolCalls) > 0 {
			cb.LogToolCalls(finalMessage.ToolCalls)

			// 添加 assistant 消息（即使 content 为空，只要有效工具调用就要加入上下文）
			if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil {
				return "", fmt.Errorf("failed to add assistant message: %w", err)
			}

			for _, res := range toolResults {
				// 只添加有效工具的结果（id 不为空）
				if res.tc.ID == "" {
					continue
				}
				if err := a.addToolResult(messageCtx, res.tc, res.result, res.err); err != nil {
					return "", fmt.Errorf("tool execution failed: %w", err)
				}
			}

			// 继续循环
			continue
		}

		// d. 没有工具调用 - 返回响应
		if content != "" {
			if err := a.ctxManager.AddMessage(messageCtx, finalMessage); err != nil {
				return "", fmt.Errorf("failed to add assistant message: %w", err)
			}
		} else {
			logger.Debug("Skipping empty assistant message")
		}

		return content, nil
	}
}

// forcedToolOptions 把用户显式点名工具转换成协议级 tool_choice。
// 调用层级：RunStreamWithOptions -> forcedToolOptions -> model.Stream/Generate。
// 主要步骤：只在输入含强制调用语义时扫描现有工具名；命中唯一完整工具名则强制该工具。
func (a *Agent) forcedToolOptions(input string) []model.Option {
	if a == nil || len(a.toolMap) == 0 || !asksForTool(input) {
		return nil
	}
	var matched []string
	for name := range a.toolMap {
		if strings.Contains(input, name) {
			matched = append(matched, name)
		}
	}
	if len(matched) != 1 {
		return nil
	}
	return []model.Option{model.WithToolChoice(schema.ToolChoiceForced, matched[0])}
}

func asksForTool(input string) bool {
	lower := strings.ToLower(input)
	return strings.Contains(lower, "must call") ||
		strings.Contains(lower, "use tool") ||
		strings.Contains(input, "必须调用") ||
		strings.Contains(input, "请调用") ||
		strings.Contains(input, "使用工具") ||
		strings.Contains(input, "调用工具")
}

// GetSkillManager 获取技能管理器
func (a *Agent) GetSkillManager() *skill.Manager {
	return a.skillManager
}

// SetModel 设置模型
func (a *Agent) SetModel(model model.ToolCallingChatModel) {
	a.model = model
}

// GetModel 返回当前绑定的 LLM 模型
// 返回: ToolCallingChatModel 实例，可能为 nil
func (a *Agent) GetModel() model.ToolCallingChatModel {
	if a == nil {
		return nil
	}
	return a.model
}

// SetTools 设置工具列表
func (a *Agent) SetTools(tools []tool.BaseTool) {
	a.tools = tools
	a.toolMap = make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		a.toolMap[info.Name] = t
	}
}

// Name 返回 Agent 名称
// 返回: Agent 配置中的名称字符串
func (a *Agent) Name() string {
	if a == nil || a.config == nil {
		return ""
	}
	return a.config.Name
}

// TokenUsage 获取当前 token 使用情况
// 返回: (已用 token 数, token 上限)
func (a *Agent) TokenUsage() (used int, limit int) {
	if a == nil || a.tokenBudget == nil {
		return 0, 0
	}
	return a.tokenBudget.Usage()
}

// 初始化，注入系统提示词，注入skill提示词。
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
