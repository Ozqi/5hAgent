package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type callbackPanicTool struct{}

func (callbackPanicTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "callback_panic_tool", Desc: "test tool"}, nil
}

func TestFormatToolErrIncludesArguments(t *testing.T) {
	errText := formatToolErr(schema.ToolCall{
		ID: "call_1",
		Function: schema.FunctionCall{
			Name:      "task.task",
			Arguments: `{"action":"finish","id":"test-tools-001"}`,
		},
	}, errors.New(`unknown action "finish": expected one of create/update/get/list/delete/archive/reopen`))

	if !strings.Contains(errText, `unknown action "finish"`) {
		t.Fatalf("formatted error missing root cause: %s", errText)
	}
	if !strings.Contains(errText, `Tool arguments sent by model: {"action":"finish","id":"test-tools-001"}`) {
		t.Fatalf("formatted error missing tool args: %s", errText)
	}
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

func TestToolCollectorWaitsForArgumentsDuringStreaming(t *testing.T) {
	idx := 0
	collector := newToolCollector()

	// 流式阶段不能把空参数立即当成 {}，因为下一片可能才是真参数。
	ready := collector.Add([]schema.ToolCall{{
		Index: &idx,
		ID:    "call_1",
		Function: schema.FunctionCall{
			Name: "mcp.notion.API-get-self",
		},
	}})
	if len(ready) != 0 {
		t.Fatalf("Add() ready calls = %d, want 0 before arguments or stream EOF", len(ready))
	}

	ready = collector.Add([]schema.ToolCall{{
		Index: &idx,
		Function: schema.FunctionCall{
			Arguments: `{"user_id":"abc"}`,
		},
	}})
	if len(ready) != 1 {
		t.Fatalf("Add() ready calls = %d, want 1 after arguments arrive", len(ready))
	}
	if ready[0].Function.Arguments != `{"user_id":"abc"}` {
		t.Fatalf("arguments = %q, want real args", ready[0].Function.Arguments)
	}

	pending := collector.PendingRunnableCalls()
	if len(pending) != 0 {
		t.Fatalf("PendingRunnableCalls() = %d, want 0 (already dispatched)", len(pending))
	}
}

func TestToolCollectorNormalizesEmptyArgumentsAtStreamEnd(t *testing.T) {
	idx := 0
	collector := newToolCollector()

	ready := collector.Add([]schema.ToolCall{{
		Index: &idx,
		ID:    "call_1",
		Function: schema.FunctionCall{
			Name: "mcp.notion.API-get-self",
		},
	}})
	if len(ready) != 0 {
		t.Fatalf("Add() ready calls = %d, want 0 before stream EOF", len(ready))
	}

	pending := collector.PendingRunnableCalls()
	if len(pending) != 1 {
		t.Fatalf("PendingRunnableCalls() = %d, want 1", len(pending))
	}
	if pending[0].Function.Arguments != "{}" {
		t.Fatalf("arguments = %q, want {}", pending[0].Function.Arguments)
	}
}
