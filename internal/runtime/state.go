package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type runtimeState struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Workspace string    `json:"workspace"`
	SessionID string    `json:"session_id"`
	Model     string    `json:"model"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *Runtime) writeRuntimeState(status string) error {
	if r == nil || r.RuntimeDir == "" {
		return nil
	}
	now := time.Now().UTC()
	created := r.CreatedAt
	if created.IsZero() {
		created = now
	}
	state := runtimeState{
		ID:        r.RuntimeID,
		Kind:      r.RuntimeKind,
		Workspace: r.ProjectDir,
		SessionID: r.SessionID,
		Model:     r.ModelRef,
		Status:    status,
		CreatedAt: created,
		UpdatedAt: now,
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	tmp, err := os.CreateTemp(r.RuntimeDir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, filepath.Join(r.RuntimeDir, "state.json"))
}
