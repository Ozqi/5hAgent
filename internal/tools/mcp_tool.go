// mcp_tool.go - MCP 工具包装器
// 功能：封装 MCP Client 调用为 Eino Tool
// 主要类型：MCPTool
// 导出函数：NewMCPTool
package tools

import (
	"context"
	"encoding/json"
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
	params := parseInputSchema(t.spec.InputSchema)
	return &schema.ToolInfo{
		Name:        mcp.FullToolName(t.serverName, t.spec.Name),
		Desc:        t.spec.Description,
		ParamsOneOf: schema.NewParamsOneOfByParams(params),
	}, nil
}

func (t *MCPTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	if t.client == nil {
		return "", fmt.Errorf("mcp client is required")
	}
	return t.client.CallTool(ctx, t.spec.Name, args)
}

var _ tool.InvokableTool = (*MCPTool)(nil)

// --- JSON Schema → Eino ParameterInfo 转换 ---

// jsonSchemaObject 表示 JSON Schema 的 object 形式
type jsonSchemaObject struct {
	Type       string                     `json:"type"`
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
	Items      json.RawMessage            `json:"items"`
}

// parseInputSchema 将 MCP 的 inputSchema (JSON Schema) 转换为 Eino ParameterInfo map
func parseInputSchema(raw json.RawMessage) map[string]*schema.ParameterInfo {
	if len(raw) == 0 {
		return map[string]*schema.ParameterInfo{}
	}

	var schemaObj jsonSchemaObject
	if err := json.Unmarshal(raw, &schemaObj); err != nil {
		return map[string]*schema.ParameterInfo{}
	}

	// 只处理 object 类型的顶层 schema（MCP 工具参数都是 object）
	if schemaObj.Type != "object" || len(schemaObj.Properties) == 0 {
		return map[string]*schema.ParameterInfo{}
	}

	// 构建 required 集合
	requiredSet := make(map[string]bool, len(schemaObj.Required))
	for _, r := range schemaObj.Required {
		requiredSet[r] = true
	}

	result := make(map[string]*schema.ParameterInfo, len(schemaObj.Properties))
	for name, propRaw := range schemaObj.Properties {
		result[name] = convertProperty(propRaw, requiredSet[name])
	}
	return result
}

// convertProperty 将单个 JSON Schema property 转换为 ParameterInfo
func convertProperty(raw json.RawMessage, required bool) *schema.ParameterInfo {
	var prop struct {
		Type        string          `json:"type"`
		Description string          `json:"description"`
		Enum        []string        `json:"enum"`
		Items       json.RawMessage `json:"items"`
		Properties  json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &prop); err != nil {
		return &schema.ParameterInfo{Type: schema.String, Desc: "", Required: required}
	}

	pi := &schema.ParameterInfo{
		Type:     schema.DataType(prop.Type),
		Desc:     prop.Description,
		Required: required,
	}

	// 处理 enum
	if len(prop.Enum) > 0 {
		pi.Enum = prop.Enum
	}

	// 处理 array 元素类型
	if prop.Type == "array" && len(prop.Items) > 0 {
		pi.ElemInfo = convertProperty(prop.Items, false)
	}

	// 处理 object 嵌套子参数
	if prop.Type == "object" && len(raw) > 0 {
		var subSchema jsonSchemaObject
		if err := json.Unmarshal(raw, &subSchema); err == nil && len(subSchema.Properties) > 0 {
			subRequired := make(map[string]bool, len(subSchema.Required))
			for _, r := range subSchema.Required {
				subRequired[r] = true
			}
			pi.SubParams = make(map[string]*schema.ParameterInfo, len(subSchema.Properties))
			for name, propRaw := range subSchema.Properties {
				pi.SubParams[name] = convertProperty(propRaw, subRequired[name])
			}
		}
	}

	return pi
}
