package systemd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestControlServerListsRunningProcesses(t *testing.T) {
	controlDir, err := os.MkdirTemp("/private/tmp", "5ha-ctl-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(controlDir)
	sys := New()
	proc := &AgentProcess{
		ID:        "agent-1",
		State:     ProcessRunning,
		StartedAt: time.Date(2026, 8, 3, 2, 0, 0, 0, time.UTC),
		SourceTask: SourceTask{
			ID:    "task-a",
			Title: "background work",
		},
	}
	proc.SetWorkLogPath("/workspace/.5hagent/agents/agent-1/logs/run.md")
	sys.processes[proc.ID] = proc

	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartControlServer(ctx, controlDir, sys, "/workspace")
	if err != nil {
		t.Fatalf("StartControlServer() error = %v", err)
	}
	defer cancel()
	defer server.Close()

	processes, err := ListProcesses(controlDir)
	if err != nil {
		t.Fatalf("ListProcesses() error = %v", err)
	}
	if len(processes) != 1 {
		t.Fatalf("processes = %#v, want one", processes)
	}
	got := processes[0]
	if !strings.HasPrefix(got.ID, "daemon-") || !strings.HasSuffix(got.ID, "/agent-1") {
		t.Fatalf("process ID = %q", got.ID)
	}
	if got.TaskID != "task-a" || got.Workspace != "/workspace" || got.WorkLogPath == "" {
		t.Fatalf("process = %#v", got)
	}

	stale := filepath.Join(controlDir, "daemon-stale.sock")
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatalf("write stale socket marker: %v", err)
	}
	if _, err := ListProcesses(controlDir); err != nil {
		t.Fatalf("ListProcesses() with stale socket error = %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale socket still exists: %v", err)
	}
}
