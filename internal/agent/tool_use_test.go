package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type callbackPanicTool struct{}

func (callbackPanicTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "callback_panic_tool", Desc: "test tool"}, nil
}

func (callbackPanicTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	return "ok", nil
}

func TestExeToolCallDebugCallbackDoesNotPanic(t *testing.T) {
	a, err := NewAgent(nil, nil, &Config{Debug: true})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	a.SetTools([]tool.BaseTool{callbackPanicTool{}})

	result, err := a.exeToolCall(context.Background(), schema.ToolCall{
		ID: "call_1",
		Function: schema.FunctionCall{
			Name:      "callback_panic_tool",
			Arguments: `{}`,
		},
	}, 0, 1, false)
	if err != nil {
		t.Fatalf("exeToolCall() error = %v", err)
	}
	if result != "ok" {
		t.Fatalf("exeToolCall() result = %q, want ok", result)
	}
}
