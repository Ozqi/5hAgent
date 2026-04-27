package utils

import (
	"sync"

	"github.com/cloudwego/eino/schema"
)

type TokenBudget struct {
	limit int
	used  int
	mu    sync.Mutex
}

func NewTokenBudget(limit int) *TokenBudget {
	return &TokenBudget{limit: limit}
}

func (b *TokenBudget) Add(meta *schema.ResponseMeta) error {
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
	// Budget tracking only – limit not enforced.
	// if b.used > b.limit {
	// 	return fmt.Errorf("max total tokens exceeded: %d > %d", b.used, b.limit)
	// }
	return nil
}

func (b *TokenBudget) Usage() (used int, limit int) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.limit
}
