package agent

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestBuildDebugLLMRequestIncludesLogicalFieldsAndRedactsSecret(t *testing.T) {
	secret := "secret-key"
	messages := []*schema.Message{
		{Role: schema.System, Content: "system"},
		{
			Role:    schema.Assistant,
			Content: "calling",
			ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "base.read_file",
					Arguments: `{"token":"secret-key"}`,
				},
			}},
		},
		{Role: schema.Tool, Name: "base.read_file", ToolCallID: "call-1", Content: "result secret-key"},
	}

	data, err := buildDebugLLMRequest(messages, "mira/gpt-test", secret, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`"model": "mira/gpt-test"`,
		`"model_options": "unavailable"`,
		`"role": "system"`,
		`"name": "base.read_file"`,
		`"tool_call_id": "call-1"`,
		`"arguments": "{\"token\":\"[REDACTED]\"}"`,
		`"content": "result [REDACTED]"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("request missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, secret) {
		t.Fatalf("request contains unredacted secret:\n%s", got)
	}
}
