package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

type fakeGenerateModel struct {
	responses []*schema.Message
	idx       int
}

func (m *fakeGenerateModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.idx >= len(m.responses) {
		return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
	}
	resp := m.responses[m.idx]
	m.idx++
	return resp, nil
}

func (m *fakeGenerateModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("unexpected Stream call")
}

func (m *fakeGenerateModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

type fakeBudgetStreamingModel struct {
	call int
}

func (m *fakeBudgetStreamingModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, fmt.Errorf("unexpected Generate call")
}

func (m *fakeBudgetStreamingModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.call++
	sr, sw := schema.Pipe[*schema.Message](2)
	go func(call int) {
		defer sw.Close()
		switch call {
		case 1:
			sw.Send(&schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{{
					ID: "budget_call_1",
					Function: schema.FunctionCall{
						Name:      "fake_tool",
						Arguments: `{"value":"ok"}`,
					},
				}},
				ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 40, CompletionTokens: 40, TotalTokens: 80}},
			}, nil)
		case 2:
			sw.Send(&schema.Message{
				Role:         schema.Assistant,
				Content:      "done",
				ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 40, CompletionTokens: 40, TotalTokens: 80}},
			}, nil)
		default:
			sw.Send(nil, io.EOF)
		}
	}(m.call)
	return sr, nil
}

func (m *fakeBudgetStreamingModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestRunFailsAfterRepeatedToolCalls(t *testing.T) {
	fakeTool := &fakeTool{}
	model := &fakeGenerateModel{responses: []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "repeat_1",
				Function: schema.FunctionCall{Name: "fake_tool", Arguments: `{"path":"a","offset":1}`},
			}},
			ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 10}},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "repeat_2",
				Function: schema.FunctionCall{Name: "fake_tool", Arguments: `{"offset":1,"path":"a"}`},
			}},
			ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 10}},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "repeat_3",
				Function: schema.FunctionCall{Name: "fake_tool", Arguments: `{"path":"a","offset":1}`},
			}},
			ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 10}},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "repeat_4",
				Function: schema.FunctionCall{Name: "fake_tool", Arguments: `{"offset":1,"path":"a"}`},
			}},
			ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 10}},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "repeat_5",
				Function: schema.FunctionCall{Name: "fake_tool", Arguments: `{"path":"a","offset":1}`},
			}},
			ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 10}},
		},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "repeat_6",
				Function: schema.FunctionCall{Name: "fake_tool", Arguments: `{"offset":1,"path":"a"}`},
			}},
			ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 10}},
		},
	}}

	ag, err := NewAgent(model, []tool.BaseTool{fakeTool}, &Config{
		Name:           "test-agent",
		MaxTotalTokens: 1000,
		SystemPrompt:   "test system prompt",
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctxManager := agentctx.NewManager()
	messageCtx, err := ctxManager.CreateContext()
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}

	_, err = ag.Run(context.Background(), messageCtx, "repeat tool")
	if err == nil || !strings.Contains(err.Error(), "repeated tool call") {
		t.Fatalf("Run() error = %v, want repeated tool call failure", err)
	}

	if fakeTool.Count() != 5 {
		t.Fatalf("tool executed %d times, want 5 before repeat failure", fakeTool.Count())
	}
}

func TestRunStreamStopsWhenTotalTokensExceeded(t *testing.T) {
	fakeTool := &fakeTool{}
	model := &fakeBudgetStreamingModel{}

	ag, err := NewAgent(model, []tool.BaseTool{fakeTool}, &Config{
		Name:           "test-agent",
		MaxTotalTokens: 100,
		SystemPrompt:   "test system prompt",
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctxManager := agentctx.NewManager()
	messageCtx, err := ctxManager.CreateContext()
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}

	_, err = ag.RunStream(context.Background(), messageCtx, "budget tool", nil)
	if err == nil || !strings.Contains(err.Error(), "max total tokens") {
		t.Fatalf("RunStream() error = %v, want max total tokens failure", err)
	}
}
