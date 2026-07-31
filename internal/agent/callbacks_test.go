package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/lzq/5hAgent/internal/utils"
)

func TestModelUsageIsTrackedWithoutDebug(t *testing.T) {
	budget := utils.NewTokenBudget(200000)
	callbacks := NewAgentCallbacks(false, budget)
	callbacks.OnModelEnd(context.Background(), nil, &model.CallbackOutput{
		TokenUsage: &model.TokenUsage{PromptTokens: 72, CompletionTokens: 8, TotalTokens: 80},
	})

	prompt, total, _ := budget.Usage()
	if prompt != 72 || total != 80 {
		t.Fatalf("Usage() = prompt %d total %d, want 72 and 80", prompt, total)
	}
}
