// tui_tool_render_test.go - 验证工具调用块首行的动作摘要。
package cli

import (
	"strings"
	"testing"
)

func TestToolIntent(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args string
		want string
	}{
		{name: "read path", tool: "base.read_file", args: `{"path":"README.md","limit":20}`, want: "read file"},
		{name: "run command", tool: "base.exec_shell", args: `{"command":"go test ./..."}`, want: "run shell command"},
		{name: "explicit action", tool: "task.task", args: `{"action":"list","status":"pending"}`, want: "list"},
		{name: "unknown tool", tool: "mcp.demo.lookup", args: `{"query":"x"}`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolIntent(tt.tool, tt.args); got != tt.want {
				t.Fatalf("toolIntent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderToolHintEntryIncludesIntentAndArgs(t *testing.T) {
	model := &AppModel{}
	got := stripANSI(model.renderToolHintEntry(conversationEntry{
		ToolName:   "read_file",
		ToolIntent: "read file",
		ToolArgs:   "[limit=20,path=README.md]",
		ToolState:  "done",
	}, 120))
	for _, want := range []string{"read_file", "read file", "[limit=20,path=README.md]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered header %q missing %q", got, want)
		}
	}
}
