package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

type callbackPanicTool struct{}

type captureRunModel struct {
	options []einomodel.Option
}

func (m *captureRunModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	m.options = append([]einomodel.Option(nil), opts...)
	return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
}

func (m *captureRunModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(msg, nil)
		sw.Close()
	}()
	return sr, nil
}

func (m *captureRunModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return m, nil
}

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

func TestMergeMessageExtraAppendsStringChunks(t *testing.T) {
	merged := mergeMessageExtra(nil, map[string]any{"thinking": "step 1 ", "count": 1})
	merged = mergeMessageExtra(merged, map[string]any{"thinking": "step 2", "count": 2})

	if got := merged["thinking"]; got != "step 1 step 2" {
		t.Fatalf("thinking extra = %v, want appended string", got)
	}
	if got := merged["count"]; got != 2 {
		t.Fatalf("count extra = %v, want latest non-string value", got)
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

func TestForcedToolOptionsForExplicitToolRequest(t *testing.T) {
	a, err := NewAgent(nil, nil, &Config{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	a.SetTools([]tool.BaseTool{callbackPanicTool{}})
	a.toolMap["base.list_dir"] = callbackPanicTool{}

	opts := a.forcedToolOptions("必须调用工具 base.list_dir，参数 path 为 .")
	if len(opts) != 1 {
		t.Fatalf("forcedToolOptions() = %d opts, want 1", len(opts))
	}
	common := einomodel.GetCommonOptions(&einomodel.Options{}, opts...)
	if common.ToolChoice == nil || *common.ToolChoice != schema.ToolChoiceForced {
		t.Fatalf("ToolChoice = %v, want forced", common.ToolChoice)
	}
	if len(common.AllowedToolNames) != 1 || common.AllowedToolNames[0] != "base.list_dir" {
		t.Fatalf("AllowedToolNames = %v, want base.list_dir", common.AllowedToolNames)
	}
}

func TestForcedToolOptionsIgnoresPlainMention(t *testing.T) {
	a, err := NewAgent(nil, nil, &Config{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	a.toolMap = map[string]tool.BaseTool{"base.list_dir": callbackPanicTool{}}

	if opts := a.forcedToolOptions("解释 base.list_dir 是什么"); len(opts) != 0 {
		t.Fatalf("forcedToolOptions() = %d opts, want 0", len(opts))
	}
}

func TestRunStreamForcesExplicitToolName(t *testing.T) {
	model := &captureRunModel{}
	manager := agentctx.NewMemoryManagerWithStore(t.TempDir())
	messageCtx, err := manager.CreateContext("")
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	a, err := NewAgent(model, nil, &Config{DisableStream: true, ContextAutoCompress: false})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	a.SetCtxManager(manager)
	a.SetTools([]tool.BaseTool{callbackPanicTool{}})
	a.toolMap["base.grep"] = callbackPanicTool{}

	_, err = a.RunStream(context.Background(), messageCtx, "必须调用工具 base.grep 搜索 OpenMontage", nil)
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	common := einomodel.GetCommonOptions(&einomodel.Options{}, model.options...)
	if common.ToolChoice == nil || *common.ToolChoice != schema.ToolChoiceForced {
		t.Fatalf("ToolChoice = %v, want forced", common.ToolChoice)
	}
	if len(common.AllowedToolNames) != 1 || common.AllowedToolNames[0] != "base.grep" {
		t.Fatalf("AllowedToolNames = %v, want base.grep", common.AllowedToolNames)
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
