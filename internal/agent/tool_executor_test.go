package agent

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestStreamToolCollectorRunnableAfterCompleteJSON(t *testing.T) {
	collector := newStreamToolCollector()

	ready := collector.Add([]schema.ToolCall{{
		ID: "call_1",
		Function: schema.FunctionCall{
			Name:      "base.read_file",
			Arguments: `{"path":"/tmp`,
		},
	}})
	if len(ready) != 0 {
		t.Fatalf("expected no runnable calls for partial json, got %d", len(ready))
	}

	ready = collector.Add([]schema.ToolCall{{
		ID: "call_1",
		Function: schema.FunctionCall{
			Arguments: `"}`,
		},
	}})
	if len(ready) != 1 {
		t.Fatalf("expected one runnable call after json completes, got %d", len(ready))
	}
	if ready[0].Function.Arguments != `{"path":"/tmp"}` {
		t.Fatalf("unexpected merged arguments: %s", ready[0].Function.Arguments)
	}
}

func TestIsRunnableToolCall(t *testing.T) {
	tests := []struct {
		name string
		call schema.ToolCall
		want bool
	}{
		{
			name: "missing id is not runnable",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "base.read_file", Arguments: `{}`}},
			want: false,
		},
		{
			name: "missing name is not runnable",
			call: schema.ToolCall{ID: "call_1", Function: schema.FunctionCall{Arguments: `{}`}},
			want: false,
		},
		{
			name: "invalid json is not runnable",
			call: schema.ToolCall{ID: "call_1", Function: schema.FunctionCall{Name: "base.read_file", Arguments: `{`}},
			want: false,
		},
		{
			name: "complete call is runnable",
			call: schema.ToolCall{ID: "call_1", Function: schema.FunctionCall{Name: "base.read_file", Arguments: `{}`}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRunnableToolCall(tt.call)
			if got != tt.want {
				t.Fatalf("isRunnableToolCall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsReadOnlyToolCall(t *testing.T) {
	tests := []struct {
		name string
		call schema.ToolCall
		want bool
	}{
		{
			name: "task list is read only",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task.task", Arguments: `{"action":"list"}`}},
			want: true,
		},
		{
			name: "task get is read only",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task.task", Arguments: `{"action":"get","id":"t1"}`}},
			want: true,
		},
		{
			name: "task create is write",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task.task", Arguments: `{"action":"create","id":"t1"}`}},
			want: false,
		},
		{
			name: "task update is write",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task.task", Arguments: `{"action":"update","id":"t1"}`}},
			want: false,
		},
		{
			name: "base grep is read only",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "base.grep", Arguments: `{"pattern":"foo"}`}},
			want: true,
		},
		{
			name: "invalid task action defaults write",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task.task", Arguments: `{"action":"wat"}`}},
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

func TestFormatToolExecutionErrorIncludesHints(t *testing.T) {
	tests := []struct {
		name string
		call schema.ToolCall
		want []string
	}{
		{
			name: "read tool suggests path fixes",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "base.read_file", Arguments: `{"path":"relative.txt"}`}},
			want: []string{"tool execution failed:", "Suggestion:", "absolute path"},
		},
		{
			name: "task tool suggests valid actions",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "task.task", Arguments: `{"action":"wat"}`}},
			want: []string{"Suggestion:", "valid task action", "create"},
		},
		{
			name: "shell tool suggests checking command",
			call: schema.ToolCall{Function: schema.FunctionCall{Name: "base.exec_shell", Arguments: `{"command":"wat"}`}},
			want: []string{"Suggestion:", "shell command", "workspace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatToolExecutionError(tt.call, assertErr("boom"))
			for _, want := range tt.want {
				if !containsString(got, want) {
					t.Fatalf("expected %q in %q", want, got)
				}
			}
		})
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func containsString(s, sub string) bool {
	return strings.Contains(s, sub)
}
