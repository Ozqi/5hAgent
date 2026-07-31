// context_tool.go - LLM 可调用的上下文管理工具
// 功能：暴露 inspect/pin/edit/audit/compress，让模型通过 tool call 管理当前 message context。
// 调用方：由 Registry.RegisterContextTool 注册；Agent.RunStream 通过 Go context 传入当前 Context。
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

const (
	contextToolName = "context.context"
	contextToolDesc = `Manage the current conversation context. Use this when the task spans many turns, before compressing history, or when important constraints must be preserved.
Actions:
- inspect: return message indexes, roles, previews, protected/pinned flags; does not return full content.
- pin: protect a message range. Requires start, end, reason.
- edit: replace one user or assistant message for the next model turn. Requires index, content, reason.
- audit: return recent context management events.
- compress: reduce context. mode=lm summarizes old history; mode=truncate keeps recent messages only.`
)

type ContextTool struct {
	llm       model.ToolCallingChatModel
	promptDir string
}

type contextToolInput struct {
	Action  string `json:"action"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Index   int    `json:"index"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
	Mode    string `json:"mode"`
}

func NewContextTool(llm model.ToolCallingChatModel, promptDir string) *ContextTool {
	return &ContextTool{llm: llm, promptDir: promptDir}
}

func (t *ContextTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: contextToolName,
		Desc: contextToolDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action":  {Type: schema.String, Desc: "Required. One of: inspect, pin, edit, audit, compress.", Required: true, Enum: []string{"inspect", "pin", "edit", "audit", "compress"}},
			"start":   {Type: schema.Integer, Desc: "Start message index. Required for pin."},
			"end":     {Type: schema.Integer, Desc: "End message index, inclusive. Required for pin."},
			"index":   {Type: schema.Integer, Desc: "Message index. Required for edit."},
			"content": {Type: schema.String, Desc: "Replacement message content. Required for edit."},
			"reason":  {Type: schema.String, Desc: "Why this context operation is needed. Required for pin and edit."},
			"mode":    {Type: schema.String, Desc: "Compress mode: lm or truncate. Default lm.", Enum: []string{"lm", "truncate"}},
		}),
	}, nil
}

func (t *ContextTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input contextToolInput
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", err
	}
	rt, ok := agentctx.ToolRuntimeFrom(ctx)
	if !ok {
		return "", fmt.Errorf("context runtime not found")
	}

	var result any
	var err error
	switch input.Action {
	case "inspect":
		result, err = rt.Manager.Inspect(rt.Context)
	case "pin":
		if input.Reason == "" {
			return "", fmt.Errorf("pin requires reason")
		}
		r := agentctx.ContextRange{Start: input.Start, End: input.End, Reason: input.Reason}
		err = rt.Manager.PinRange(rt.Context, r)
		result = map[string]any{"ok": err == nil, "action": "pin", "range": r}
	case "edit":
		err = rt.Manager.EditMessage(rt.Context, input.Index, input.Content, input.Reason)
		result = map[string]any{"ok": err == nil, "action": "edit", "index": input.Index}
	case "audit":
		events := rt.Manager.Audit(rt.Context)
		if len(events) > 20 {
			events = events[len(events)-20:]
		}
		result = events
	case "compress":
		mode := input.Mode
		if mode == "" {
			mode = "lm"
		}
		before, after, compressErr := 0, 0, error(nil)
		if mode == "truncate" {
			before, after, compressErr = rt.Manager.Compress(rt.Context)
		} else if mode == "lm" {
			before, after, compressErr = rt.Manager.LMCompress(ctx, rt.Context, t.llm, t.promptDir)
		} else {
			return "", fmt.Errorf("invalid compress mode: %s", mode)
		}
		err = compressErr
		result = map[string]any{"ok": err == nil, "action": "compress", "mode": mode, "before": before, "after": after}
	default:
		return "", fmt.Errorf("invalid context action: %s", input.Action)
	}
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

var _ tool.InvokableTool = (*ContextTool)(nil)
