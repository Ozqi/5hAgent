package utils

import (
	"sync"

	"github.com/cloudwego/eino/components/model"
)

// TokenBudget 保存一次 Agent 生命周期中的 token 预算和累计使用量。
type TokenBudget struct {
	limit         int
	lastPrompt    int
	sessionTotal  int
	contextWindow int
	mu            sync.Mutex
}

// NewTokenBudget 创建具有给定总量上限的 token 计数器。
func NewTokenBudget(limit int) *TokenBudget {
	return &TokenBudget{limit: limit}
}

// AddUsage 记录最近 prompt token，并将本次总 token 累加到会话总量。
// TotalTokens 缺失时使用 PromptTokens 与 CompletionTokens 之和。
func (b *TokenBudget) AddUsage(usage *model.TokenUsage) {
	if b == nil || usage == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastPrompt = usage.PromptTokens
	if usage.TotalTokens > 0 {
		b.sessionTotal += usage.TotalTokens
	} else {
		b.sessionTotal += usage.PromptTokens + usage.CompletionTokens
	}
}

// Usage 返回最近 prompt token、会话累计 token 和上下文窗口。
func (b *TokenBudget) Usage() (prompt int, total int, window int) {
	if b == nil {
		return 0, 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastPrompt, b.sessionTotal, b.contextWindow
}

// SetContextWindow 更新当前模型的上下文窗口。
func (b *TokenBudget) SetContextWindow(window int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.contextWindow = window
	b.mu.Unlock()
}

// SessionTotal 返回会话累计 token 数。
func (b *TokenBudget) SessionTotal() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionTotal
}
