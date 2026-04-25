package agent

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

type fakeStreamingModel struct {
	t           *testing.T
	mu          sync.Mutex
	streamCalls int
	seenMsgs    [][]*schema.Message
}

func (m *fakeStreamingModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, fmt.Errorf("unexpected Generate call")
}

func (m *fakeStreamingModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.mu.Lock()
	m.streamCalls++
	call := m.streamCalls
	m.seenMsgs = append(m.seenMsgs, cloneMessages(input))
	m.mu.Unlock()

	sr, sw := schema.Pipe[*schema.Message](4)
	go func() {
		defer sw.Close()

		switch call {
		case 1:
			sw.Send(&schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{{
					ID: "call_1",
					Function: schema.FunctionCall{
						Name:      "fake_tool",
						Arguments: `{"value":"ok"}`,
					},
				}},
			}, nil)
			// Leave enough time for the old async path to accidentally execute the tool.
			time.Sleep(50 * time.Millisecond)
		case 2:
			if got := countToolMessages(input, "call_1"); got != 1 {
				sw.Send(nil, fmt.Errorf("expected exactly 1 tool result for call_1 before second stream, got %d", got))
				return
			}
			if !assistantBeforeTool(input, "call_1") {
				sw.Send(nil, fmt.Errorf("tool result for call_1 appeared before assistant tool call message"))
				return
			}
			sw.Send(&schema.Message{Role: schema.Assistant, Content: "done"}, nil)
		default:
			sw.Send(nil, io.EOF)
		}
	}()

	return sr, nil
}

func (m *fakeStreamingModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

type fakeTool struct {
	mu          sync.Mutex
	invocations int
}

func (t *fakeTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "fake_tool", Desc: "fake tool"}, nil
}

func (t *fakeTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.invocations++
	return `{"status":"ok"}`, nil
}

func (t *fakeTool) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.invocations
}

func TestRunStreamExecutesEachToolCallOnce(t *testing.T) {
	fakeModel := &fakeStreamingModel{t: t}
	fakeTool := &fakeTool{}

	ag, err := NewAgent(fakeModel, []tool.BaseTool{fakeTool}, &Config{
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

	got, err := ag.RunStream(context.Background(), messageCtx, "trigger tool", nil)
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if got != "done" {
		t.Fatalf("RunStream() = %q, want done", got)
	}

	if fakeTool.Count() != 1 {
		t.Fatalf("tool executed %d times, want 1", fakeTool.Count())
	}
}

func countToolMessages(messages []*schema.Message, toolCallID string) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == schema.Tool && msg.ToolCallID == toolCallID {
			count++
		}
	}
	return count
}

func assistantBeforeTool(messages []*schema.Message, toolCallID string) bool {
	assistantIdx := -1
	toolIdx := -1

	for i, msg := range messages {
		if assistantIdx == -1 {
			for _, tc := range msg.ToolCalls {
				if tc.ID == toolCallID {
					assistantIdx = i
					break
				}
			}
		}
		if toolIdx == -1 && msg.Role == schema.Tool && msg.ToolCallID == toolCallID {
			toolIdx = i
		}
	}

	return assistantIdx >= 0 && toolIdx > assistantIdx
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	cloned := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		msgCopy := *msg
		if len(msg.ToolCalls) > 0 {
			msgCopy.ToolCalls = append([]schema.ToolCall(nil), msg.ToolCalls...)
		}
		cloned = append(cloned, &msgCopy)
	}
	return cloned
}
