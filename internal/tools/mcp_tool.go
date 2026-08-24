package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ozqi/walle/internal/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// MCPTool 是单个远端 MCP 工具的 Eino 适配器。
type MCPTool struct {
	serverName string
	client     mcp.Client
	spec       mcp.ToolSpec
}

// NewMCPTool 使用指定 MCP client 和 schema 创建工具包装器。
func NewMCPTool(serverName string, client mcp.Client, spec mcp.ToolSpec) *MCPTool {
	return &MCPTool{serverName: serverName, client: client, spec: spec}
}

// Info 将 MCP JSON Schema 的受支持子集转换为 Eino 参数信息。
func (t *MCPTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	params := parseInputSchema(t.spec.InputSchema)
	return &schema.ToolInfo{
		Name:        mcp.FullToolName(t.serverName, t.spec.Name),
		Desc:        t.spec.Description,
		ParamsOneOf: schema.NewParamsOneOfByParams(params),
	}, nil
}

// InvokableRun 将模型生成的原始 JSON 参数转发给进程外 MCP server。
func (t *MCPTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	// MCP client 是外部信任边界；参数校验和副作用约束最终由远端工具负责。
	if t.client == nil {
		return "", fmt.Errorf("mcp client is required")
	}
	return t.client.CallTool(ctx, t.spec.Name, args)
}

var _ tool.InvokableTool = (*MCPTool)(nil)

// --- JSON Schema → Eino ParameterInfo 转换 ---

// jsonSchemaObject 表示 JSON Schema 的 object 形式
type jsonSchemaObject struct {
	Type       json.RawMessage            `json:"type"`
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

	// MCP 工具参数通常是 object；部分 OpenAPI MCP schema 省略顶层 type。
	if inferJSONSchemaType(schemaObj.Type, schemaObj.Properties, schemaObj.Items) != "object" || len(schemaObj.Properties) == 0 {
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

// convertProperty 将 MCP/OpenAPI 的 JSON Schema 子集转换为 ParameterInfo。
// 当前只处理 type/enum/const/items/properties/required；oneOf/allOf/anyOf 等组合 schema 先保持忽略。
func convertProperty(raw json.RawMessage, required bool) *schema.ParameterInfo {
	var prop struct {
		Type        json.RawMessage `json:"type"`
		Description string          `json:"description"`
		Enum        []string        `json:"enum"`
		Const       interface{}     `json:"const"`
		Items       json.RawMessage `json:"items"`
		Properties  json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &prop); err != nil {
		return &schema.ParameterInfo{Type: schema.String, Desc: "", Required: required}
	}
	dataType := inferJSONSchemaType(prop.Type, nil, prop.Items)
	if dataType == "" && len(prop.Properties) > 0 {
		dataType = "object"
	}
	if dataType == "" {
		dataType = "string"
	}
	desc := strings.TrimSpace(prop.Description)
	if required && desc == "" {
		desc = "Required parameter."
	}

	pi := &schema.ParameterInfo{
		Type:     schema.DataType(dataType),
		Desc:     desc,
		Required: required,
	}

	if len(prop.Enum) > 0 {
		pi.Enum = prop.Enum
	}
	if prop.Const != nil {
		pi.Enum = []string{fmt.Sprint(prop.Const)}
		if pi.Desc == "" || pi.Desc == "Required parameter." {
			pi.Desc = "Must be " + fmt.Sprint(prop.Const)
		}
	}

	if dataType == "array" && len(prop.Items) > 0 {
		pi.ElemInfo = convertProperty(prop.Items, false)
	}

	if dataType == "object" && len(raw) > 0 {
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

func inferJSONSchemaType(raw json.RawMessage, properties map[string]json.RawMessage, items json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		if len(properties) > 0 {
			return "object"
		}
		if len(items) > 0 {
			return "array"
		}
		return ""
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return normalizeJSONSchemaType(single)
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		for _, candidate := range many {
			candidate = normalizeJSONSchemaType(candidate)
			if candidate != "" && candidate != "null" {
				return candidate
			}
		}
	}
	return "string"
}

func normalizeJSONSchemaType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "integer":
		return "number"
	case "null":
		return ""
	default:
		return strings.ToLower(strings.TrimSpace(t))
	}
}
