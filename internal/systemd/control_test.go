package systemd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeInteractiveProcess struct {
	stopCalled chan struct{}
}

func (p *fakeInteractiveProcess) Snapshot() ProcessSnapshot {
	return ProcessSnapshot{ID: "interactive", State: ProcessRunning, StartedAt: time.Now().UTC(), Interactive: true}
}

func (p *fakeInteractiveProcess) Attach() ([]ProcessEvent, <-chan ProcessEvent, func()) {
	events := make(chan ProcessEvent)
	return nil, events, func() { close(events) }
}

func (p *fakeInteractiveProcess) Submit(string) error { return nil }

func (p *fakeInteractiveProcess) Stop() error {
	close(p.stopCalled)
	return nil
}

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

	stale := filepath.Join(controlDir, "daemon-stale.sock")
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatalf("write stale socket marker: %v", err)
	}

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
	if got.ID != "agent-1" {
		t.Fatalf("process ID = %q", got.ID)
	}
	if got.TaskID != "task-a" || got.Workspace != "/workspace" || got.WorkLogPath == "" {
		t.Fatalf("process = %#v", got)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale socket still exists: %v", err)
	}
}

func TestListProcessesRemovesStaleSupervisorSocket(t *testing.T) {
	controlDir, err := os.MkdirTemp("/private/tmp", "5ha-ctl-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(controlDir)
	stale := filepath.Join(controlDir, supervisorSocket)
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatalf("write stale supervisor socket: %v", err)
	}
	processes, err := ListProcesses(controlDir)
	if err != nil {
		t.Fatalf("ListProcesses() error = %v", err)
	}
	if len(processes) != 0 {
		t.Fatalf("processes = %#v, want none", processes)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale supervisor socket still exists: %v", err)
	}
}

func TestProcessClientStopCallsInteractiveProcess(t *testing.T) {
	controlDir, err := os.MkdirTemp("/private/tmp", "5ha-ctl-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(controlDir)
	sys := New()
	proc := &fakeInteractiveProcess{stopCalled: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartControlServer(ctx, controlDir, sys, "/workspace", proc)
	if err != nil {
		t.Fatalf("StartControlServer() error = %v", err)
	}
	defer cancel()
	defer server.Close()

	client, err := AttachProcess(controlDir, "interactive")
	if err != nil {
		t.Fatalf("AttachProcess() error = %v", err)
	}
	defer client.Close()
	if err := client.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-proc.stopCalled:
	case <-time.After(time.Second):
		t.Fatal("interactive process was not stopped")
	}
}
