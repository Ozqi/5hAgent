package utils

import (
	"sync"

	"github.com/cloudwego/eino/components/model"
)

type TokenBudget struct {
	limit         int
	lastPrompt    int
	sessionTotal  int
	contextWindow int
	mu            sync.Mutex
}

func NewTokenBudget(limit int) *TokenBudget {
	return &TokenBudget{limit: limit}
}

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

func (b *TokenBudget) Usage() (prompt int, total int, window int) {
	if b == nil {
		return 0, 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastPrompt, b.sessionTotal, b.contextWindow
}

func (b *TokenBudget) SetContextWindow(window int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.contextWindow = window
	b.mu.Unlock()
}

func (b *TokenBudget) SessionTotal() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionTotal
}
