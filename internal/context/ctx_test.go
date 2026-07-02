package context

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestInspectReturnsMessageIndexAndPinnedFlags(t *testing.T) {
	mgr := NewManager()
	ctx, err := mgr.CreateContext("")
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}

	if err := mgr.AddMessage(ctx, &schema.Message{Role: schema.System, Content: "system prompt"}); err != nil {
		t.Fatalf("AddMessage(system) error = %v", err)
	}
	if err := mgr.AddMessage(ctx, &schema.Message{Role: schema.User, Content: strings.Repeat("u", 120)}); err != nil {
		t.Fatalf("AddMessage(user) error = %v", err)
	}
	if err := mgr.AddMessage(ctx, &schema.Message{Role: schema.Tool, Content: "tool output"}); err != nil {
		t.Fatalf("AddMessage(tool) error = %v", err)
	}
	if err := mgr.PinRange(ctx, ContextRange{Start: 1, End: 1, Reason: "user requirement"}); err != nil {
		t.Fatalf("PinRange() error = %v", err)
	}

	inspect, err := mgr.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if inspect.MessageCount != 3 {
		t.Fatalf("Inspect().MessageCount = %d, want 3", inspect.MessageCount)
	}
	if inspect.EstimatedChars != len("system prompt")+120+len("tool output") {
		t.Fatalf("Inspect().EstimatedChars = %d", inspect.EstimatedChars)
	}
	if len(inspect.Messages) != 3 {
		t.Fatalf("len(Inspect().Messages) = %d, want 3", len(inspect.Messages))
	}
	if inspect.Messages[0].Index != 0 || inspect.Messages[0].Role != string(schema.System) {
		t.Fatalf("first inspected message = %+v", inspect.Messages[0])
	}
	if !hasFlag(inspect.Messages[0].Flags, "protected") {
		t.Fatalf("system message flags = %v, want protected", inspect.Messages[0].Flags)
	}
	if !hasFlag(inspect.Messages[1].Flags, "pinned") {
		t.Fatalf("pinned message flags = %v, want pinned", inspect.Messages[1].Flags)
	}
	if len(inspect.Messages[1].Preview) >= 120 {
		t.Fatalf("preview was not truncated: %q", inspect.Messages[1].Preview)
	}
	if len(inspect.Pinned) != 1 || inspect.Pinned[0].Reason != "user requirement" {
		t.Fatalf("Inspect().Pinned = %+v", inspect.Pinned)
	}
}

func TestValidateEditableRangeRejectsProtectedRecentAndPinnedMessages(t *testing.T) {
	mgr := NewManager()
	ctx, err := mgr.CreateContext("")
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if err := mgr.AddMessage(ctx, &schema.Message{Role: schema.System, Content: "system prompt"}); err != nil {
		t.Fatalf("AddMessage(system) error = %v", err)
	}
	for i := 0; i < KeepRecentMessages+4; i++ {
		if err := mgr.AddMessage(ctx, &schema.Message{Role: schema.User, Content: "history"}); err != nil {
			t.Fatalf("AddMessage(history %d) error = %v", i, err)
		}
	}

	if err := mgr.ValidateEditableRange(ctx, ContextRange{Start: 1, End: 3}); err != nil {
		t.Fatalf("ValidateEditableRange(editable old range) error = %v", err)
	}
	if err := mgr.ValidateEditableRange(ctx, ContextRange{Start: 0, End: 1}); err == nil {
		t.Fatalf("ValidateEditableRange(system range) error = nil, want error")
	}
	if err := mgr.ValidateEditableRange(ctx, ContextRange{Start: 5, End: 6}); err == nil {
		t.Fatalf("ValidateEditableRange(recent range) error = nil, want error")
	}
	if err := mgr.PinRange(ctx, ContextRange{Start: 2, End: 2, Reason: "keep"}); err != nil {
		t.Fatalf("PinRange() error = %v", err)
	}
	if err := mgr.ValidateEditableRange(ctx, ContextRange{Start: 1, End: 3}); err == nil {
		t.Fatalf("ValidateEditableRange(pinned range) error = nil, want error")
	}
}

func TestAuditRecordsPinEvents(t *testing.T) {
	mgr := NewManager()
	ctx, err := mgr.CreateContext("")
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if err := mgr.AddMessage(ctx, &schema.Message{Role: schema.User, Content: "remember this"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	if err := mgr.PinRange(ctx, ContextRange{Start: 0, End: 0, Reason: "important"}); err != nil {
		t.Fatalf("PinRange() error = %v", err)
	}

	events := mgr.Audit(ctx)
	if len(events) != 1 {
		t.Fatalf("len(Audit()) = %d, want 1", len(events))
	}
	if events[0].Op != "pin" || events[0].Range.Start != 0 || events[0].Range.End != 0 || events[0].Range.Reason != "important" {
		t.Fatalf("Audit()[0] = %+v", events[0])
	}
	if events[0].CreatedAt.IsZero() {
		t.Fatalf("Audit()[0].CreatedAt is zero")
	}
}

func hasFlag(flags []string, want string) bool {
	for _, flag := range flags {
		if flag == want {
			return true
		}
	}
	return false
}
