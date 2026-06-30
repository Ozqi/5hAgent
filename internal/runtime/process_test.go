package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/systemd"
	"github.com/lzq/5hAgent/internal/task"
)

type captureModel struct {
	messages []*schema.Message
	options  []einomodel.Option
}

func (m *captureModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	m.messages = append([]*schema.Message(nil), input...)
	m.options = append([]einomodel.Option(nil), opts...)
	return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
}

func (m *captureModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(msg, nil)
		sw.Close()
	}()
	return sr, nil
}

func (m *captureModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return m, nil
}

func TestRunProcessInjectsPromptSpec(t *testing.T) {
	model := &captureModel{}
	ag, err := agent.NewAgent(model, nil, &agent.Config{
		Name:                "test-agent",
		MaxTotalTokens:      1000000,
		RepeatToolLimit:     5,
		ContextAutoCompress: false,
		SystemPrompt:        "",
		ProjectDataDir:      t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	rt := &Runtime{
		Agent:      ag,
		CtxManager: agentctx.NewMemoryManagerWithStore(t.TempDir()),
		ProjectDir: t.TempDir(),
	}
	ag.SetCtxManager(rt.CtxManager)

	err = rt.RunProcess(context.Background(), &systemd.AgentProcess{
		ID: "agent-test",
		Spec: systemd.ProcessSpec{
			Prompt: systemd.PromptSpec{
				System: "process system",
				Skills: []systemd.SkillRef{{
					Name:        "debugging",
					Description: "debug carefully",
				}},
			},
			Exit: systemd.ExitSpec{Condition: "finish"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("RunProcess() error = %v", err)
	}
	if len(model.messages) < 3 {
		t.Fatalf("messages = %d, want system prompt, skill prompt, user input", len(model.messages))
	}
	if model.messages[0].Role != schema.System || model.messages[0].Content != "process system" {
		t.Fatalf("first message = %+v, want process system", model.messages[0])
	}
	if model.messages[1].Role != schema.System || !strings.Contains(model.messages[1].Content, "# Skill: debugging") || !strings.Contains(model.messages[1].Content, "debug carefully") {
		t.Fatalf("skill message = %+v, want skill summary", model.messages[1])
	}
}

func TestRunProcessUpdatesSourceTaskAndReportTrace(t *testing.T) {
	model := &captureModel{}
	projectDir := t.TempDir()
	list, err := task.NewTaskList(filepath.Join(projectDir, ".5hagent", "task.md"))
	if err != nil {
		t.Fatalf("NewTaskList() error = %v", err)
	}
	if _, err := list.CreateTask("task-a", "Task A", "finish A"); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	ag, err := agent.NewAgent(model, nil, &agent.Config{
		Name:                "test-agent",
		MaxTotalTokens:      1000000,
		RepeatToolLimit:     5,
		ContextAutoCompress: false,
		SystemPrompt:        "",
		ProjectDataDir:      filepath.Join(projectDir, ".5hagent"),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	rt := &Runtime{
		Agent:      ag,
		TaskList:   list,
		CtxManager: agentctx.NewMemoryManagerWithStore(t.TempDir()),
		ProjectDir: projectDir,
	}
	ag.SetCtxManager(rt.CtxManager)

	proc := &systemd.AgentProcess{
		ID:         "agent-test",
		Name:       "agent-test",
		SourceTask: systemd.SourceTask{ID: "task-a", Title: "Task A", EventID: "event-a"},
		Spec: systemd.ProcessSpec{
			Prompt: systemd.PromptSpec{System: "process system"},
			Exit:   systemd.ExitSpec{Condition: "finish"},
		},
	}
	if err := rt.RunProcess(context.Background(), proc, nil); err != nil {
		t.Fatalf("RunProcess() error = %v", err)
	}
	updated, err := list.GetTask("task-a")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if updated.Status != task.StatusCompleted {
		t.Fatalf("task status = %s, want completed", updated.Status)
	}
	if proc.ReportPath == "" || proc.WorkLogPath == "" {
		t.Fatalf("report/worklog not set: report=%q worklog=%q", proc.ReportPath, proc.WorkLogPath)
	}
	if !strings.Contains(filepath.Base(proc.ReportPath), "task-a.agent-test.") {
		t.Fatalf("report path = %s, want task/process/timestamp name", proc.ReportPath)
	}
	report, err := os.ReadFile(proc.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	text := string(report)
	for _, want := range []string{"- task: task-a", "- task_title: Task A", "- source_event: event-a", "- worklog: " + proc.WorkLogPath} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

func TestRunProcessForbidsToolsWhenTaskSaysNoTools(t *testing.T) {
	model := &captureModel{}
	plainModel := &captureModel{}
	projectDir := t.TempDir()
	list, err := task.NewTaskList(filepath.Join(projectDir, ".5hagent", "task.md"))
	if err != nil {
		t.Fatalf("NewTaskList() error = %v", err)
	}
	if _, err := list.CreateTask("task-a", "Task A", "不要调用工具"); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	ag, err := agent.NewAgent(model, nil, &agent.Config{
		Name:                "test-agent",
		MaxTotalTokens:      1000000,
		RepeatToolLimit:     5,
		ContextAutoCompress: false,
		SystemPrompt:        "",
		ProjectDataDir:      filepath.Join(projectDir, ".5hagent"),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	rt := &Runtime{
		Agent:         ag,
		TaskList:      list,
		CtxManager:    agentctx.NewMemoryManagerWithStore(t.TempDir()),
		ProjectDir:    projectDir,
		decisionModel: plainModel,
	}
	ag.SetCtxManager(rt.CtxManager)
	proc := &systemd.AgentProcess{
		ID:         "agent-test",
		Name:       "agent-test",
		SourceTask: systemd.SourceTask{ID: "task-a", Title: "Task A", EventID: "event-a"},
		Spec: systemd.ProcessSpec{
			Prompt: systemd.PromptSpec{System: "process system"},
			Exit:   systemd.ExitSpec{Condition: "finish"},
		},
	}
	if err := rt.RunProcess(context.Background(), proc, nil); err != nil {
		t.Fatalf("RunProcess() error = %v", err)
	}
	if len(model.messages) != 0 {
		t.Fatalf("tool-bound model was used, messages=%d", len(model.messages))
	}
	if len(plainModel.messages) == 0 {
		t.Fatal("plain model was not used")
	}
	if ag.GetModel() != model {
		t.Fatal("agent model was not restored after no-tool process")
	}
	opts := einomodel.GetCommonOptions(&einomodel.Options{}, plainModel.options...)
	if opts.ToolChoice == nil || *opts.ToolChoice != schema.ToolChoiceForbidden {
		t.Fatalf("ToolChoice = %v, want forbidden", opts.ToolChoice)
	}
}

func TestProcessReportNameDoesNotOverwrite(t *testing.T) {
	first := processReportName(&processReport{ProcessID: "agent-1", Source: systemd.SourceTask{ID: "task-a"}})
	second := processReportName(&processReport{ProcessID: "agent-1", Source: systemd.SourceTask{ID: "task-a"}})
	if first == second {
		t.Fatalf("processReportName returned duplicate %q", first)
	}
	if !strings.HasPrefix(first, "task-a.agent-1.") {
		t.Fatalf("name = %s, want task/process prefix", first)
	}
}

func TestTaskFileEventSourceSkipsCompletedTask(t *testing.T) {
	dir := t.TempDir()
	list, err := task.NewTaskList(filepath.Join(dir, "task.md"))
	if err != nil {
		t.Fatalf("NewTaskList() error = %v", err)
	}
	if _, err := list.CreateTask("task-a", "Task A", "finish A"); err != nil {
		t.Fatalf("CreateTask(task-a) error = %v", err)
	}
	if _, err := list.CreateTask("task-b", "Task B", "finish B"); err != nil {
		t.Fatalf("CreateTask(task-b) error = %v", err)
	}
	if err := list.UpdateTaskStatus("task-a", task.StatusCompleted); err != nil {
		t.Fatalf("UpdateTaskStatus(task-a) error = %v", err)
	}
	source := NewTaskFileEventSource(list, time.Millisecond, func(t *task.Task) (systemd.ProcessSpec, error) {
		return systemd.ProcessSpec{Prompt: systemd.PromptSpec{System: t.ID}, Exit: systemd.ExitSpec{Condition: "done"}}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = list.UpdateTaskStatus("task-b", task.StatusInProgress)
	}()
	event, err := source.Next(ctx)
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	var payload systemd.TaskCreatedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TaskID != "task-b" || payload.TaskTitle != "Task B" {
		t.Fatalf("payload task = %s/%s, want task-b/Task B", payload.TaskID, payload.TaskTitle)
	}
	if payload.ProcessSpec.Prompt.System != "task-b" {
		t.Fatalf("prompt = %q, want task-b", payload.ProcessSpec.Prompt.System)
	}
}
