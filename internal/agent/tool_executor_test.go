package agent

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestIsReadOnlyToolCall(t *testing.T) {
	tests := []struct {
		name string
		call schema.ToolCall
		want bool
	}{
		{
			name: "task list is read only",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task", Arguments: `{"action":"list"}`}},
			want: true,
		},
		{
			name: "task get is read only",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task", Arguments: `{"action":"get","id":"t1"}`}},
			want: true,
		},
		{
			name: "task create is write",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task", Arguments: `{"action":"create","id":"t1"}`}},
			want: false,
		},
		{
			name: "task update is write",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task", Arguments: `{"action":"update","id":"t1"}`}},
			want: false,
		},
		{
			name: "old task_list stays read only",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task_list"}},
			want: true,
		},
		{
			name: "invalid task action defaults write",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task", Arguments: `{"action":"wat"}`}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isReadOnlyToolCall(tt.call)
			if got != tt.want {
				t.Fatalf("isReadOnlyToolCall() = %v, want %v", got, tt.want)
			}
		})
	}
}
