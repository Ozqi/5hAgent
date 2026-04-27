package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockMCPServer 一个简单的模拟 MCP 服务器用于测试
func startMockServer() (stdinReader *strings.Reader, stdoutWriter *strings.Builder, cmdCtx context.Context, cmdCancel func()) {
	cmdCtx, cmdCancel = context.WithCancel(context.Background())
	stdinReader = &strings.Reader{}
	stdoutWriter = &strings.Builder{}
	return
}

// TestStdioClient_Basic 测试基本功能
func TestStdioClient_Basic(t *testing.T) {
	// 跳过，因为需要真实的 MCP 服务器
	t.Skip("需要真实的 MCP 服务器进行集成测试")
}

// TestJSONRPCRequest 验证 JSON-RPC 请求格式
func TestJSONRPCRequest(t *testing.T) {
	req := jsonrpcRequest{
		Jsonrpc: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test_tool","arguments":{}}`),
		ID:      1,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed jsonrpcRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Jsonrpc != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", parsed.Jsonrpc)
	}
	if parsed.Method != "tools/call" {
		t.Errorf("expected method tools/call, got %s", parsed.Method)
	}
}

// TestToolSpecParsing 测试工具规格解析
func TestToolSpecParsing(t *testing.T) {
	// 测试新格式 { tools: [...] }
	newFormat := `{
		"tools": [
			{"name": "tool1", "description": "desc1", "readOnly": true},
			{"name": "tool2", "description": "desc2", "readOnly": false}
		]
	}`

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(newFormat), &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools array")
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// 测试旧格式 [ ... ]
	oldFormat := `[
		{"name": "tool1", "description": "desc1"},
		{"name": "tool2", "description": "desc2"}
	]`

	var oldTools []ToolSpec
	if err := json.Unmarshal([]byte(oldFormat), &oldTools); err != nil {
		t.Fatalf("unmarshal old format failed: %v", err)
	}

	if len(oldTools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(oldTools))
	}
}

// TestStdioClientConfig 测试配置验证
func TestStdioClientConfig(t *testing.T) {
	config := StdioClientConfig{
		Name:           "test_server",
		Command:        "npx",
		Args:           []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		StartupTimeout: 5 * time.Second,
	}

	if config.Name == "" {
		t.Error("name should not be empty")
	}
	if config.Command == "" {
		t.Error("command should not be empty")
	}
	if config.StartupTimeout == 0 {
		t.Error("startup timeout should have default value or be set")
	}
}

// TestJSONRPCResponseParsing 测试响应解析
func TestJSONRPCResponseParsing(t *testing.T) {
	// 成功响应
	successResp := `{
		"jsonrpc": "2.0",
		"result": {
			"content": [
				{"type": "text", "text": "hello world"}
			]
		},
		"id": 1
	}`

	var resp jsonrpcResponse
	if err := json.Unmarshal([]byte(successResp), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if resp.Error != nil {
		t.Error("expected no error")
	}
	if resp.Result == nil {
		t.Error("expected result")
	}

	// 错误响应
	errorResp := `{
		"jsonrpc": "2.0",
		"error": {
			"code": -32600,
			"message": "Invalid Request"
		},
		"id": 1
	}`

	var errResp jsonrpcResponse
	if err := json.Unmarshal([]byte(errorResp), &errResp); err != nil {
		t.Fatalf("unmarshal error response failed: %v", err)
	}

	if errResp.Error == nil {
		t.Error("expected error")
	}
	if errResp.Error.Code != -32600 {
		t.Errorf("expected code -32600, got %d", errResp.Error.Code)
	}
}

// TestServerConfigValidation 测试服务器配置验证
func TestServerConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  ServerConfig
		wantErr bool
	}{
		{
			name: "valid",
			config: ServerConfig{
				Name:    "test_server",
				Command: "npx",
			},
			wantErr: false,
		},
		{
			name: "empty name",
			config: ServerConfig{
				Name:    "",
				Command: "npx",
			},
			wantErr: true,
		},
		{
			name: "invalid name",
			config: ServerConfig{
				Name:    "test-server",
				Command: "npx",
			},
			wantErr: true,
		},
		{
			name: "empty command",
			config: ServerConfig{
				Name:    "test_server",
				Command: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ExampleFullToolName 演示工具名生成
func ExampleFullToolName() {
	name := FullToolName("filesystem", "read_file")
	fmt.Println(name)
	// Output: mcp.filesystem.read_file
}
