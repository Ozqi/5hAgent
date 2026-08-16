package systemd

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

type fakeRunner struct {
	err error
}

func (r fakeRunner) RunProcess(ctx context.Context, proc *AgentProcess) error {
	return r.err
}

type blockingRunner struct {
	started chan struct{}
	release chan struct{}
}

type flakySource struct {
	calls int
}

func (s *flakySource) Next(ctx context.Context) (Event, error) {
	s.calls++
	if s.calls == 1 {
		return Event{}, &os.PathError{Op: "stat", Path: "missing", Err: os.ErrNotExist}
	}
	<-time.After(10 * time.Millisecond)
	return Event{ID: "ok", Type: "file.changed", Source: "test"}, nil
}

func (r blockingRunner) RunProcess(ctx context.Context, proc *AgentProcess) error {
	close(r.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

func validSpec() ProcessSpec {
	return ProcessSpec{
		SystemPrompt:  "run",
		ExitCondition: "done",
	}
}

func TestRunProcessEmitsExited(t *testing.T) {
	sys := New()
	proc, err := sys.RunProcess(context.Background(), fakeRunner{}, validSpec())
	if err != nil {
		t.Fatalf("RunProcess() error = %v", err)
	}
	if proc.State != ProcessExited {
		t.Fatalf("state = %s, want %s", proc.State, ProcessExited)
	}
	event, ok := sys.nextEvent(context.Background())
	if !ok {
		t.Fatal("nextEvent() not ok")
	}
	if event.Type != "process.exited" || event.ProcessID != proc.ID {
		t.Fatalf("event = %+v, want process.exited for %s", event, proc.ID)
	}
}

func TestRunProcessRejectsConcurrentProcess(t *testing.T) {
	sys := New()
	runner := blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := sys.RunProcess(context.Background(), runner, validSpec())
		done <- err
	}()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("first process did not start")
	}
	if _, err := sys.RunProcess(context.Background(), fakeRunner{}, validSpec()); err == nil {
		t.Fatal("RunProcess() error = nil, want concurrent process rejection")
	}
	close(runner.release)
	if err := <-done; err != nil {
		t.Fatalf("first RunProcess() error = %v", err)
	}
}

func TestDispatchInvalidTaskSpecEmitsFailed(t *testing.T) {
	sys := New()
	payload, err := json.Marshal(TaskCreatedPayload{ProcessSpec: ProcessSpec{}, TaskID: "bad-task", TaskTitle: "Bad Task"})
	if err != nil {
		t.Fatal(err)
	}
	err = sys.dispatch(context.Background(), fakeRunner{}, Event{
		ID:      "bad",
		Type:    "task.created",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("dispatch() error = %v", err)
	}
	event, ok := sys.nextEvent(context.Background())
	if !ok {
		t.Fatal("nextEvent() not ok")
	}
	if event.Type != "process.failed" || event.ID != "bad.failed" {
		t.Fatalf("event = %+v, want bad.failed process.failed", event)
	}
}

func TestTaskCreatedPayloadStrictSchema(t *testing.T) {
	spec := validSpec()
	payload, err := json.Marshal(TaskCreatedPayload{ProcessSpec: spec, TaskID: "task-1", TaskTitle: "Task"})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := processStartFromEvent(Event{Type: "task.created", Payload: payload})
	if err != nil {
		t.Fatalf("processStartFromEvent() error = %v", err)
	}
	if got.SystemPrompt != spec.SystemPrompt {
		t.Fatalf("spec = %+v, want %+v", got, spec)
	}

	badPayload := []byte(`{"process_spec":{"system_prompt":"run","exit_condition":"done"},"unknown":true}`)
	if _, _, err := processStartFromEvent(Event{Type: "task.created", Payload: badPayload}); err == nil {
		t.Fatal("processStartFromEvent() error = nil, want unknown field error")
	}
}

func TestTaskCreatedPayloadKeepsSourceTrace(t *testing.T) {
	spec := validSpec()
	payload, err := json.Marshal(TaskCreatedPayload{ProcessSpec: spec, TaskID: "task-1", TaskTitle: "Task"})
	if err != nil {
		t.Fatal(err)
	}
	got, source, err := processStartFromEvent(Event{ID: "event-1", Type: "task.created", Payload: payload})
	if err != nil {
		t.Fatalf("processStartFromEvent() error = %v", err)
	}
	if got.SystemPrompt != spec.SystemPrompt {
		t.Fatalf("spec = %+v, want %+v", got, spec)
	}
	if source.ID != "task-1" || source.Title != "Task" || source.EventID != "event-1" {
		t.Fatalf("source = %+v, want task trace", source)
	}
}

func TestStartSourceContinuesAfterRecoverableError(t *testing.T) {
	sys := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &flakySource{}

	sys.StartSource(ctx, source)
	event, ok := sys.nextEvent(ctx)
	if !ok {
		t.Fatal("nextEvent() not ok")
	}
	if event.ID != "ok" || source.calls < 2 {
		t.Fatalf("event=%+v calls=%d, want recovered event after retry", event, source.calls)
	}
}
