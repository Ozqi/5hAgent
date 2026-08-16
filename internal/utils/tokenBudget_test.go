package utils

import (
	"testing"

	"github.com/cloudwego/eino/components/model"
)

func TestTokenBudgetTracksPromptTotalAndWindow(t *testing.T) {
	budget := NewTokenBudget(200000)
	budget.SetContextWindow(32768)
	budget.AddUsage(&model.TokenUsage{PromptTokens: 72, CompletionTokens: 8, TotalTokens: 80})
	budget.AddUsage(&model.TokenUsage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110})

	prompt, total, window := budget.Usage()
	if prompt != 100 || total != 190 || window != 32768 {
		t.Fatalf("Usage() = %d, %d, %d; want 100, 190, 32768", prompt, total, window)
	}
}
