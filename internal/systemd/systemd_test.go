package systemd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lzq/5hAgent/internal/ipctypes"
)

type fakeRunner struct {
	err error
}

func (r fakeRunner) RunProcess(ctx context.Context, proc *AgentProcess, ipc IPC) error {
	return r.err
}

type blockingRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r blockingRunner) RunProcess(ctx context.Context, proc *AgentProcess, ipc IPC) error {
	close(r.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

type fakeCaller struct {
	out []byte
}

func (c fakeCaller) CallDecision(ctx context.Context, input []byte) ([]byte, error) {
	return c.out, nil
}

func validSpec() ProcessSpec {
	return ProcessSpec{
		Prompt: PromptSpec{System: "run"},
		Exit:   ExitSpec{Condition: "done"},
	}
}

func eventPayload(t *testing.T, spec ProcessSpec) []byte {
	t.Helper()
	payload, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return payload
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

func TestNewAppliesOptions(t *testing.T) {
	sys := New(WithMaxRetry(3), WithStalledAfter(time.Second))
	input := sys.decisionInput(Event{Type: "task.failed"})
	if input.Policy.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want 3", input.Policy.MaxRetries)
	}
	if input.Policy.StalledAfter != time.Second.String() {
		t.Fatalf("StalledAfter = %s, want %s", input.Policy.StalledAfter, time.Second)
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

func TestDispatchHighRiskDoesNotStart(t *testing.T) {
	sys := New()
	err := sys.dispatch(context.Background(), fakeRunner{}, Event{
		ID:      "risk",
		Type:    "process.start",
		Risk:    "high",
		Payload: eventPayload(t, validSpec()),
	})
	if err != nil {
		t.Fatalf("dispatch() error = %v", err)
	}
	if got := sys.ListProcesses(); len(got) != 0 {
		t.Fatalf("processes = %d, want 0", len(got))
	}
}

func TestDispatchInvalidSpecEmitsFailed(t *testing.T) {
	sys := New()
	err := sys.dispatch(context.Background(), fakeRunner{}, Event{
		ID:      "bad",
		Type:    "process.start",
		Payload: eventPayload(t, ProcessSpec{}),
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

func TestIPCMessageRoundTrip(t *testing.T) {
	sys := New()
	sys.processes["agent-1"] = &AgentProcess{ID: "agent-1", State: ProcessRunning}
	msg := ipctypes.Message{From: "agent-0", To: "agent-1", Summary: "hello"}
	if err := sys.Send(msg); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	got, err := sys.Recv("agent-1")
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if len(got) != 1 || got[0].From != "agent-0" || got[0].Summary != "hello" {
		t.Fatalf("messages = %+v, want one ipc message", got)
	}
	again, err := sys.Recv("agent-1")
	if err != nil {
		t.Fatalf("second Recv() error = %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("messages after recv = %+v, want empty mailbox", again)
	}
}

func TestIPCRejectsInvalidMessage(t *testing.T) {
	sys := New()
	sys.processes["agent-1"] = &AgentProcess{ID: "agent-1", State: ProcessRunning}
	if err := sys.Send(ipctypes.Message{Summary: "missing target"}); err == nil {
		t.Fatal("Send() error = nil, want missing target error")
	}
	if err := sys.Send(ipctypes.Message{To: "agent-404", Summary: "missing process"}); err == nil {
		t.Fatal("Send() error = nil, want missing process error")
	}
	if err := sys.Send(ipctypes.Message{To: "agent-1"}); err == nil {
		t.Fatal("Send() error = nil, want empty message error")
	}
}

func TestRunRetriesFailedTaskOnce(t *testing.T) {
	sys := New()
	decision, err := json.Marshal(DecisionResult{Action: DecisionRetry, Spec: validSpec()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sys.Emit(Event{ID: "fail-1", Type: "task.failed", Source: "task-a"})
		sys.Emit(Event{ID: "fail-2", Type: "task.failed", Source: "task-a"})
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err = sys.Run(ctx, fakeRunner{}, fakeCaller{out: decision})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := len(sys.ListProcesses()); got != 1 {
		t.Fatalf("processes = %d, want 1 retry process", got)
	}
}

func TestParseDecisionRejectsInvalidJSON(t *testing.T) {
	if _, err := ParseDecision([]byte(`{"action":"start_agent","process_spec":{"prompt":{},"exit":{}}}`)); err == nil {
		t.Fatal("ParseDecision() error = nil, want invalid spec error")
	}
	if _, err := ParseDecision([]byte(`{"action":"stop_agent"}`)); err == nil {
		t.Fatal("ParseDecision() error = nil, want missing stop target error")
	}
	if _, err := ParseDecision([]byte(`{"action":"start_agent","process_spec":{"prompt":{"system":"run","skills":[{"description":"missing name"}]},"exit":{"condition":"done"}}}`)); err == nil {
		t.Fatal("ParseDecision() error = nil, want missing skill name error")
	}
	if _, err := ParseDecision([]byte(`{"action":"unknown"}`)); err == nil {
		t.Fatal("ParseDecision() error = nil, want invalid action error")
	}
	if _, err := ParseDecision([]byte(`{"action":"wait","extra":true}`)); err == nil {
		t.Fatal("ParseDecision() error = nil, want unknown field error")
	}
}

func TestTaskCreatedPayloadStrictSchema(t *testing.T) {
	spec := validSpec()
	payload, err := json.Marshal(TaskCreatedPayload{ProcessSpec: spec, TaskID: "task-1", TaskTitle: "Task"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := processSpecFromEvent(Event{Type: "task.created", Payload: payload})
	if err != nil {
		t.Fatalf("processSpecFromEvent() error = %v", err)
	}
	if got.Prompt.System != spec.Prompt.System {
		t.Fatalf("spec = %+v, want %+v", got, spec)
	}

	badPayload := []byte(`{"process_spec":{"prompt":{"system":"run"},"exit":{"condition":"done"}},"unknown":true}`)
	if _, err := processSpecFromEvent(Event{Type: "task.created", Payload: badPayload}); err == nil {
		t.Fatal("processSpecFromEvent() error = nil, want unknown field error")
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
	if got.Prompt.System != spec.Prompt.System {
		t.Fatalf("spec = %+v, want %+v", got, spec)
	}
	if source.ID != "task-1" || source.Title != "Task" || source.EventID != "event-1" {
		t.Fatalf("source = %+v, want task trace", source)
	}
}

func TestTimerTickDoesNotEnterSeen(t *testing.T) {
	sys := New()
	sys.Emit(Event{ID: "tick-1", Type: "timer.tick"})
	sys.Emit(Event{ID: "tick-1", Type: "timer.tick"})
	if sys.seen["tick-1"] {
		t.Fatal("timer.tick should not be recorded in seen")
	}
	if len(sys.events) != 2 {
		t.Fatalf("events = %d, want duplicate timer ticks queued", len(sys.events))
	}
}

func TestProcessEndClearsRetryKeys(t *testing.T) {
	sys := New()
	sys.retries["task-a"] = 1
	sys.retries["agent-1"] = 1
	sys.processes["agent-1"] = &AgentProcess{ID: "agent-1", State: ProcessRunning}

	sys.applyEvent(Event{Type: "process.exited", Source: "task-a", ProcessID: "agent-1"})

	if _, ok := sys.retries["task-a"]; ok {
		t.Fatal("retry key task-a was not cleared")
	}
	if _, ok := sys.retries["agent-1"]; ok {
		t.Fatal("retry key agent-1 was not cleared")
	}
}
