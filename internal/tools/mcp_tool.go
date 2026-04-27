// mcp_tool.go - MCP 工具包装器
// 功能：封装 MCP Client 调用为 Eino Tool
// 主要类型：MCPTool
// 导出函数：NewMCPTool
package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/mcp"
)

type MCPTool struct {
	serverName string
	client     mcp.Client
	spec       mcp.ToolSpec
}

func NewMCPTool(serverName string, client mcp.Client, spec mcp.ToolSpec) *MCPTool {
	return &MCPTool{serverName: serverName, client: client, spec: spec}
}

func (t *MCPTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        mcp.FullToolName(t.serverName, t.spec.Name),
		Desc:        t.spec.Description,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *MCPTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	if t.client == nil {
		return "", fmt.Errorf("mcp client is required")
	}
	return t.client.CallTool(ctx, t.spec.Name, args)
}

var _ tool.InvokableTool = (*MCPTool)(nil)
