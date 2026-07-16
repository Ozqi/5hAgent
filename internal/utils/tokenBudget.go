package utils

import (
	"sync"

	"github.com/cloudwego/eino/components/model"
)

type TokenBudget struct {
	limit        int
	used         int
	sessionTotal int
	mu           sync.Mutex
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
	if usage.TotalTokens > 0 {
		b.sessionTotal += usage.TotalTokens
	} else {
		b.sessionTotal += usage.PromptTokens + usage.CompletionTokens
	}
}

func (b *TokenBudget) Usage() (used int, limit int) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.limit
}

func (b *TokenBudget) SessionTotal() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionTotal
}
