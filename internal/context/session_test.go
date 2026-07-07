package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestSessionPersistsMessageCreatedAt(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	session, err := store.GetOrCreate("test-session")
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	msg := &schema.Message{Role: schema.User, Content: "hello"}
	if err := store.Append(session, msg); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if _, ok := msg.Extra["created_at"].(string); !ok {
		t.Fatalf("message extra = %#v, want created_at", msg.Extra)
	}
	data, err := os.ReadFile(filepath.Join(store.GetSessionDir(), "test-session.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `"created_at"`) {
		t.Fatalf("session file = %s, want message created_at", string(data))
	}

	loaded, err := store.LoadMessages(session)
	if err != nil {
		t.Fatalf("LoadMessages() error = %v", err)
	}
	createdAt, ok := loaded[0].Extra["created_at"].(string)
	if !ok {
		t.Fatalf("loaded extra = %#v, want created_at", loaded[0].Extra)
	}
	if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
		t.Fatalf("created_at = %q, want RFC3339: %v", createdAt, err)
	}
}
