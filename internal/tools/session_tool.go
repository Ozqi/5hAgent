// session_tool.go - Agent Systemd session 系统工具
// 功能：让 Agent 显式创建、保存、解除当前内存 context 的持久化 session。
// 调用方：由 tools.InitRegistry 注册为 sys.session；Agent.RunStream 通过 Go context 传入当前 Context。
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	agentctx "github.com/lzq/5hAgent/internal/context"
)

const (
	sessionToolName = "sys.session"
	sessionToolDesc = `Persist the current Agent process context only when explicitly needed.
Actions:
- create: bind current memory context to a persistent session. Optional session_id.
- save: write current context to the bound session.
- drop: unbind current context from its session without deleting files.`
)

type sessionToolInput struct {
	Action    string `json:"action"`
	SessionID string `json:"session_id"`
}

type SessionTool struct{}

func NewSessionTool() *SessionTool {
	return &SessionTool{}
}

func (t *SessionTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: sessionToolName,
		Desc: sessionToolDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action":     {Type: schema.String, Desc: "Required. One of: create, save, drop.", Required: true, Enum: []string{"create", "save", "drop"}},
			"session_id": {Type: schema.String, Desc: "Optional session id for create."},
		}),
	}, nil
}

func (t *SessionTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input sessionToolInput
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", err
	}
	rt, ok := agentctx.ToolRuntimeFrom(ctx)
	if !ok {
		return "", fmt.Errorf("context runtime not found")
	}
	switch input.Action {
	case "create":
		id, err := rt.Manager.BindSession(rt.Context, input.SessionID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"ok":true,"action":"create","session_id":%q}`, id), nil
	case "save":
		if err := rt.Manager.SaveSession(rt.Context); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"ok":true,"action":"save","session_id":%q}`, rt.Manager.GetSessionID(rt.Context)), nil
	case "drop":
		oldID := rt.Manager.GetSessionID(rt.Context)
		if err := rt.Manager.DropSession(rt.Context); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"ok":true,"action":"drop","session_id":%q}`, oldID), nil
	}
	return "", fmt.Errorf("invalid session action: %s", input.Action)
}

var _ tool.InvokableTool = (*SessionTool)(nil)
