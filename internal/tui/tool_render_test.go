// tui_tool_render_test.go - 验证工具调用块首行的动作摘要。
package tui

import (
	"encoding/json"
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

func TestSummarizeEditDiffTrimsAndLimits(t *testing.T) {
	long := strings.Repeat("a", 200)
	args, _ := json.Marshal(map[string]string{
		"old_string": "same\n" + long + "\nold2\nold3\nold4\ntail",
		"new_string": "same\nnew1\nnew2\nnew3\nnew4\ntail",
	})
	want := strings.Join([]string{
		truncateMiddle("- "+long, 160), "- old2", "- old3", "- ...",
		"+ new1", "+ new2", "+ new3", "+ ...",
	}, "\n")
	if got := summarizeEditDiff(string(args)); got != want {
		t.Fatalf("summarizeEditDiff() = %q, want %q", got, want)
	}
}
