package logger

import (
	"errors"
	"strings"
	"testing"
)

func TestPrintToolErrorSendsShortSummaryToSink(t *testing.T) {
	var event ToolEvent
	SetToolEventSink(func(e ToolEvent) {
		event = e
	})
	defer SetToolEventSink(nil)

	PrintToolError("base.edit", `{}`, errors.New("[EnhancedLocalFunc] failed to invoke tool, toolName=base.edit, err=failed to read file '/home/lzq/Proj/5hWorkSpace/tool_test.txt': open /home/lzq/Proj/5hWorkSpace/tool_test.txt: no such file or directory. Make sure the path is correct and the file exists. Use absolute paths like '/home/user/project/file.py'"))

	if event.Kind != "error" || event.Name != "base.edit" {
		t.Fatalf("event = %#v, want base.edit error event", event)
	}
	if strings.Contains(event.Text, "[EnhancedLocalFunc]") || strings.Contains(event.Text, "Use absolute paths") {
		t.Fatalf("event text = %q, want short UI summary without wrapped details", event.Text)
	}
	if !strings.Contains(event.Text, "edit failed") || !strings.Contains(event.Text, "tool_test.txt") || !strings.Contains(event.Text, "no such file or directory") {
		t.Fatalf("event text = %q, want concise edit failure with path and reason", event.Text)
	}
}
