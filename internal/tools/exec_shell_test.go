package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestExecShellCapturesStderrOnSuccess(t *testing.T) {
	candidate, err := NewExecShellTool(t.TempDir())
	if err != nil {
		t.Fatalf("NewExecShellTool() error = %v", err)
	}
	result, err := candidate.InvokableRun(context.Background(), &schema.ToolArgument{Text: `{"command":"printf out; printf warn >&2"}`})
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	var output ExecShellOutput
	if len(result.Parts) == 0 {
		t.Fatalf("tool result has no parts")
	}
	if err := json.Unmarshal([]byte(result.Parts[0].Text), &output); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", result.Parts[0].Text, err)
	}
	if output.Stdout != "out" {
		t.Fatalf("stdout = %q, want out", output.Stdout)
	}
	if !strings.Contains(output.Stderr, "warn") {
		t.Fatalf("stderr = %q, want warn", output.Stderr)
	}
	if output.ReturnCode != 0 {
		t.Fatalf("returncode = %d, want 0", output.ReturnCode)
	}
}
