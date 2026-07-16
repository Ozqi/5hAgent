// systemd.go - Agent Systemd 顶层调度骨架
// 功能：定义 Agent 进程、启动提示词、退出条件、事件和最小 task supervisor 调度循环。
// 调用方：由上层创建调度器，由 internal/runtime 作为 ProcessRunner 执行真实 Agent。
// 全局变量：无。AgentSystemd 的状态应放在结构体实例内，避免多调度器互相污染。
package systemd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// =============================================================================
// 数据契约：进程、事件、启动规格
// =============================================================================

// ProcessState 表示 AgentProcess 生命周期状态。
type ProcessState string

const (
	ProcessRunning ProcessState = "running"
	ProcessExited  ProcessState = "exited"
	ProcessFailed  ProcessState = "failed"
)

// ProcessSpec 是启动 AgentProcess 的唯一输入。
type ProcessSpec struct {
	SystemPrompt  string `json:"system_prompt"`            // system prompt
	ExitCondition string `json:"exit_condition,omitempty"` // 自然语言退出条件
}

// Event 是 Agent Systemd 调度循环处理的事实输入。
type Event struct {
	ID        string          `json:"id"`         // 事件 ID，用于去重
	Type      string          `json:"type"`       // 事件类型，如 task.created/process.exited
	Source    string          `json:"source"`     // 事件来源
	ProcessID string          `json:"process_id"` // 目标进程 ID，可为空表示广播事件
	Payload   json.RawMessage `json:"payload"`    // 结构化负载
	CreatedAt time.Time       `json:"created_at"` // 创建时间
}

// FileEventPayload 是文件 watcher 事件的最小负载。
type FileEventPayload struct {
	Path    string    `json:"path"`     // 被观察文件路径
	ModTime time.Time `json:"mod_time"` // 文件修改时间
	Size    int64     `json:"size"`     // 文件大小
}

// TaskCreatedPayload 是 task.created 事件的负载。
type TaskCreatedPayload struct {
	ProcessSpec ProcessSpec `json:"process_spec"` // 启动规格
	TaskID      string      `json:"task_id"`      // 任务 ID
	TaskTitle   string      `json:"task_title"`   // 任务标题
}

// SourceTask 记录 AgentProcess 来源任务，不进入 ProcessSpec。
type SourceTask struct {
	ID      string `json:"id"`       // task.md 中的任务 ID
	Title   string `json:"title"`    // task.md 中的任务标题
	EventID string `json:"event_id"` // 触发本进程的 task.created 事件 ID
}

// EventSource 是外部事件源的最小接口。
type EventSource interface {
	Next(ctx context.Context) (Event, error)
}

// FileEventSource 轮询单个文件并在 mtime/size 变化时产生事件。
type FileEventSource struct {
	Path        string        // 被观察文件路径
	EventType   string        // 变化时产生的事件类型
	Source      string        // 事件来源
	Interval    time.Duration // 轮询间隔
	lastModTime time.Time
	lastSize    int64
	ready       bool
}

// NewFileEventSource 创建单文件轮询事件源。
// 参数：path 是文件路径；eventType 为空时用 file.changed；source 为空时用 file。
// 调用层级：外部入口 -> NewFileEventSource -> StartSource -> Run。
// 步骤：保存配置；首次 Next 只建立基线，后续 mtime/size 变化才返回事件。
func NewFileEventSource(path string, eventType string, source string, interval time.Duration) *FileEventSource {
	if eventType == "" {
		eventType = "file.changed"
	}
	if source == "" {
		source = "file"
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &FileEventSource{Path: path, EventType: eventType, Source: source, Interval: interval}
}

// =============================================================================
// 调度器状态：进程表、事件队列
// =============================================================================

// AgentProcess 是一个运行中的 Agent 进程记录。
type AgentProcess struct {
	ID          string       // 进程 ID
	Name        string       // 进程名
	State       ProcessState // 生命周期状态
	Spec        ProcessSpec  // 启动提示词和退出条件
	StartedAt   time.Time    // 启动时间
	EndedAt     time.Time    // 结束时间
	SourceTask  SourceTask   // 来源任务；为空表示非 task 事件启动
	ReportPath  string       // 进程报告路径
	WorkLogPath string       // 进程工作日志路径
	cancel      context.CancelFunc
}

// ProcessRunner 是 AgentProcess 的执行层接口。
type ProcessRunner interface {
	RunProcess(ctx context.Context, proc *AgentProcess) error
}

// AgentSystemd 管理 AgentProcess 生命周期。
// 必须通过 New 创建；cond 和内部 map 依赖 New 完成初始化。
type AgentSystemd struct {
	mu        sync.Mutex               // 保护进程表和 nextPID
	cond      *sync.Cond               // 等待事件队列
	processes map[string]*AgentProcess // 进程表；后续实现前不得外泄可变引用
	events    []Event                  // 内存事件队列
	seen      map[string]bool          // 已入队事件 ID，用于去重
	nextPID   int                      // 本地递增进程号
}

// New 创建 AgentSystemd。
// 参数：无。
// 调用层级：外部入口 -> New -> Run/RunProcess。
// 步骤：初始化进程表；不读取配置，不创建 runtime，不调用 LLM。
func New() *AgentSystemd {
	s := &AgentSystemd{processes: map[string]*AgentProcess{}, seen: map[string]bool{}, nextPID: 1}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// =============================================================================
// 主循环：消费事件并触发硬编码调度
// =============================================================================

// Run 启动调度循环。
// 参数：ctx 控制调度器生命周期；runner 执行 AgentProcess。
// 调用层级：外部入口 -> Run -> dispatch -> RunProcess。
// 步骤：接收事件 -> 按事件类型调度 -> 等待下一事件。
func (s *AgentSystemd) Run(ctx context.Context, runner ProcessRunner) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.cond.Broadcast()
			s.mu.Unlock()
		case <-done:
		}
	}()
	defer close(done)
	for {
		event, ok := s.nextEvent(ctx)
		if !ok {
			return ctx.Err()
		}
		if err := s.dispatch(ctx, runner, event); err != nil {
			return err
		}
	}
}

// =============================================================================
// 事件解析和状态变更
// =============================================================================

func validateSpec(spec ProcessSpec) error {
	if spec.SystemPrompt == "" {
		return fmt.Errorf("prompt system is required")
	}
	if spec.ExitCondition == "" {
		return fmt.Errorf("exit condition is required")
	}
	return nil
}

// 必须在持有 AgentSystemd.mu 时调用。
func (proc *AgentProcess) terminate(state ProcessState) {
	proc.State = state
	if proc.cancel != nil {
		proc.cancel()
		proc.cancel = nil
	}
	if proc.EndedAt.IsZero() {
		proc.EndedAt = time.Now().UTC()
	}
}

func (s *AgentSystemd) dispatch(ctx context.Context, runner ProcessRunner, event Event) error {
	if event.Type == "task.created" {
		if runner == nil {
			return fmt.Errorf("process runner is required")
		}
		spec, source, err := processStartFromEvent(event)
		if err != nil {
			return fmt.Errorf("parse %s payload: %w", event.Type, err)
		}
		go func() {
			proc, err := s.RunProcessWithSource(ctx, runner, spec, source)
			if err == nil || proc != nil {
				return
			}
			id := ""
			if event.ID != "" {
				id = event.ID + ".failed"
			}
			s.Emit(Event{ID: id, Type: "process.failed", Source: "systemd.dispatch", CreatedAt: time.Now().UTC()})
		}()
		return nil
	}
	s.applyEvent(event)
	return nil
}

func processStartFromEvent(event Event) (ProcessSpec, SourceTask, error) {
	var payload TaskCreatedPayload
	if err := decodeStrict(event.Payload, &payload); err != nil {
		return ProcessSpec{}, SourceTask{}, err
	}
	return payload.ProcessSpec, SourceTask{ID: payload.TaskID, Title: payload.TaskTitle, EventID: event.ID}, nil
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *AgentSystemd) nextEvent(ctx context.Context) (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.events) == 0 {
		if ctx.Err() != nil {
			return Event{}, false
		}
		s.cond.Wait()
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, true
}

func (s *AgentSystemd) applyEvent(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, proc := range s.processes {
		if proc == nil {
			continue
		}
		if event.ProcessID != "" && event.ProcessID != proc.ID {
			continue
		}
		switch event.Type {
		case "process.exited":
			proc.terminate(ProcessExited)
		case "process.failed":
			proc.terminate(ProcessFailed)
		}
	}
}

// =============================================================================
// 事件源：内存队列、文件 watcher
// =============================================================================

// Emit 写入一个调度事件。
// 参数：event 是外部或内部事实；ID 可为空。
// 调用层级：外部 watcher / Runtime runner / 系统工具 -> Emit -> Run。
// 步骤：按非空 ID 去重 -> 补 CreatedAt -> 追加到内存事件队列。
func (s *AgentSystemd) Emit(event Event) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.ID != "" {
		if s.seen[event.ID] {
			return
		}
		s.seen[event.ID] = true
	}
	s.events = append(s.events, event)
	s.cond.Signal()
}

// StartSource 启动外部事件源。
// 参数：source 是 watcher 之外的事件生产者。
// 调用层级：外部入口 -> StartSource -> Emit -> Run。
// 步骤：循环读取 source.Next；正常事件写入队列；ctx 取消时退出。
func (s *AgentSystemd) StartSource(ctx context.Context, source EventSource) {
	if source == nil {
		return
	}
	go func() {
		for {
			event, err := source.Next(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if isRecoverableSourceError(err) {
					continue
				}
				return
			}
			s.Emit(event)
		}
	}()
}

func isRecoverableSourceError(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission)
}

// Next 等待文件变化并返回一个事件。
// 参数：ctx 控制等待生命周期。
// 调用层级：StartSource -> FileEventSource.Next -> os.Stat。
// 步骤：按 Interval 轮询文件状态 -> 首次建立基线 -> 变化时返回 file event。
func (w *FileEventSource) Next(ctx context.Context) (Event, error) {
	if w == nil || w.Path == "" {
		return Event{}, fmt.Errorf("file event source path is required")
	}
	interval := w.Interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		case <-ticker.C:
			event, ok, err := w.poll()
			if err != nil {
				return Event{}, err
			}
			if ok {
				return event, nil
			}
		}
	}
}

func (w *FileEventSource) poll() (Event, bool, error) {
	info, err := os.Stat(w.Path)
	if err != nil {
		return Event{}, false, fmt.Errorf("stat watched file %s: %w", w.Path, err)
	}
	modTime := info.ModTime().UTC()
	size := info.Size()
	if !w.ready {
		w.lastModTime = modTime
		w.lastSize = size
		w.ready = true
		return Event{}, false, nil
	}
	if modTime.Equal(w.lastModTime) && size == w.lastSize {
		return Event{}, false, nil
	}
	w.lastModTime = modTime
	w.lastSize = size
	payload, err := json.Marshal(FileEventPayload{Path: w.Path, ModTime: modTime, Size: size})
	if err != nil {
		return Event{}, false, fmt.Errorf("marshal file event payload: %w", err)
	}
	id := fmt.Sprintf("%s:%s:%d", w.Path, modTime.Format(time.RFC3339Nano), size)
	return Event{ID: id, Type: w.EventType, Source: w.Source, Payload: payload, CreatedAt: time.Now().UTC()}, true, nil
}

// =============================================================================
// 进程执行：串行创建 AgentProcess，并把执行交给外部 runner
// =============================================================================

// RunProcess 同步运行一个 AgentProcess。
// 参数：ctx 控制执行生命周期；runner 是外部执行引擎；spec 只包含 SystemPrompt/ExitCondition。
// 调用层级：Agent Systemd 单进程调度 -> RunProcess -> runner.RunProcess。
// 步骤：创建进程记录 -> 标记 running -> 调用 runner -> 按错误标记状态 -> 投递结束事件。
func (s *AgentSystemd) RunProcess(ctx context.Context, runner ProcessRunner, spec ProcessSpec) (*AgentProcess, error) {
	return s.RunProcessWithSource(ctx, runner, spec, SourceTask{})
}

// RunProcessWithSource 同步运行带来源追踪的 AgentProcess。
// 参数：source 只记录调度来源，不参与进程启动 prompt。
// 调用层级：dispatch(task.created) -> RunProcessWithSource -> runner.RunProcess。
// 步骤：校验启动规格 -> 创建进程记录和 source trace -> 调 runner -> 投递结束事件。
func (s *AgentSystemd) RunProcessWithSource(ctx context.Context, runner ProcessRunner, spec ProcessSpec, source SourceTask) (*AgentProcess, error) {
	if runner == nil {
		return nil, fmt.Errorf("process runner is required")
	}
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	s.mu.Lock()
	for _, existing := range s.processes {
		if existing != nil && existing.State == ProcessRunning {
			s.mu.Unlock()
			return nil, fmt.Errorf("another process is already running")
		}
	}
	id := fmt.Sprintf("agent-%d", s.nextPID)
	s.nextPID++
	now := time.Now().UTC()
	runCtx, cancel := context.WithCancel(ctx)
	proc := &AgentProcess{
		ID:         id,
		Name:       id,
		State:      ProcessRunning,
		Spec:       spec,
		StartedAt:  now,
		SourceTask: source,
		cancel:     cancel,
	}
	s.processes[id] = proc
	s.mu.Unlock()

	err := runner.RunProcess(runCtx, proc)
	s.mu.Lock()
	eventType := "process.exited"
	if err != nil {
		proc.terminate(ProcessFailed)
		endedAt := proc.EndedAt
		eventType = "process.failed"
		s.mu.Unlock()
		s.Emit(Event{Type: eventType, Source: "systemd", ProcessID: proc.ID, CreatedAt: endedAt})
		return proc, err
	}
	proc.terminate(ProcessExited)
	endedAt := proc.EndedAt
	s.mu.Unlock()
	s.Emit(Event{Type: eventType, Source: "systemd", ProcessID: proc.ID, CreatedAt: endedAt})
	return proc, nil
}
