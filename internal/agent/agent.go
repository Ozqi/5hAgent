// Package agent 提供 AI Agent 的核心实现
package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/miniAgent/internal/context"
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

		// a. 获取所有消息
		messages, err := a.ctxManager.GetMessages(messageCtx)
		if err != nil {
			return "", fmt.Errorf("failed to get messages: %w", err)
		}

		// b. 调用 LLM 生成响应
		resp, err := a.model.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("LLM generation failed: %w", err)
		}

		// c. 检查是否有工具调用
		if len(resp.ToolCalls) > 0 {
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
	for _, tc := range toolCalls {
		// 查找工具
		t := a.findTool(tc.Function.Name)
		if t == nil {
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
		result, err := invokable.InvokableRun(ctx, tc.Function.Arguments)
		if err != nil {
			// 工具执行失败
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
		resultMsg := schema.ToolMessage(result, tc.ID)
		if err := a.ctxManager.AddMessage(messageCtx, resultMsg); err != nil {
			return fmt.Errorf("failed to add tool result: %w", err)
		}
	}

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
