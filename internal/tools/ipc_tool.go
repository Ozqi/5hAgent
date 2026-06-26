// ipc_tool.go - Agent Systemd IPC 系统工具
// 功能：让 Agent 发送/接收短 IPC 消息，不共享上下文。
// 调用方：由 Registry.Init 注册为 sys.ipc；Agent.RunStream 通过 Go context 传入进程身份。
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
	ipcToolName = "sys.ipc"
	ipcToolDesc = `Send or receive short messages between Agent processes. Does not share context.
Actions:
- send: requires to and summary or artifact.
- recv: receive messages for current process.`
)

type ipcToolInput struct {
	Action   string `json:"action"`
	To       string `json:"to"`
	Summary  string `json:"summary"`
	Artifact string `json:"artifact"`
}

type IPCTool struct{}

func NewIPCTool() *IPCTool {
	return &IPCTool{}
}

func (t *IPCTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ipcToolName,
		Desc: ipcToolDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action":   {Type: schema.String, Desc: "Required. One of: send, recv.", Required: true, Enum: []string{"send", "recv"}},
			"to":       {Type: schema.String, Desc: "Target process id. Required for send."},
			"summary":  {Type: schema.String, Desc: "Short message summary for send."},
			"artifact": {Type: schema.String, Desc: "Optional artifact path for large content."},
		}),
	}, nil
}

func (t *IPCTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var input ipcToolInput
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", err
	}
	rt, ok := agentctx.ToolRuntimeFrom(ctx)
	if !ok {
		return "", fmt.Errorf("context runtime not found")
	}
	if rt.ProcessID == "" || rt.IPC == nil {
		return "", fmt.Errorf("ipc runtime not found")
	}
	switch input.Action {
	case "send":
		if input.To == "" {
			return "", fmt.Errorf("send requires to")
		}
		if err := rt.IPC.SendIPC(rt.ProcessID, input.To, input.Summary, input.Artifact); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"ok":true,"action":"send","from":%q,"to":%q}`, rt.ProcessID, input.To), nil
	case "recv":
		messages, err := rt.IPC.RecvIPC(rt.ProcessID)
		if err != nil {
			return "", err
		}
		data, _ := json.Marshal(map[string]any{"ok": true, "action": "recv", "messages": messages})
		return string(data), nil
	}
	return "", fmt.Errorf("invalid ipc action: %s", input.Action)
}

var _ tool.InvokableTool = (*IPCTool)(nil)
