// Package agentd 管理本地 Agent 进程调度、事件队列和 Unix Socket 控制通道。
package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// =============================================================================
// 数据契约：进程、事件、启动规格
// =============================================================================

// ProcessState 表示 AgentProcess 生命周期状态。
type ProcessState string

const (
	// ProcessIdle 表示进程尚未执行。
	ProcessIdle ProcessState = "idle"
	// ProcessRunning 表示 runner 正在执行进程。
	ProcessRunning ProcessState = "running"
	// ProcessExited 表示进程正常结束。
	ProcessExited ProcessState = "exited"
	// ProcessFailed 表示进程执行失败或被取消。
	ProcessFailed ProcessState = "failed"
)

// ProcessSpec 是启动 AgentProcess 的唯一输入。
type ProcessSpec struct {
	SystemPrompt  string `json:"system_prompt"`            // system prompt
	ExitCondition string `json:"exit_condition,omitempty"` // 自然语言退出条件
}

// Event 是 Agentd 调度循环处理的事实输入。
type Event struct {
	ID        string          `json:"id"`         // 事件 ID，用于去重
	Type      string          `json:"type"`       // 事件类型，如 process.start/process.exited
	Source    string          `json:"source"`     // 事件来源
	ProcessID string          `json:"process_id"` // 目标进程 ID，可为空表示广播事件
	Payload   json.RawMessage `json:"payload"`    // 结构化负载
	CreatedAt time.Time       `json:"created_at"` // 创建时间
}

// ProcessStartPayload 是 process.start 事件的严格负载。
type ProcessStartPayload struct {
	ProcessSpec ProcessSpec `json:"process_spec"` // 启动规格
}

// =============================================================================
// 调度器状态：进程表、事件队列
// =============================================================================

// AgentProcess 是一个运行中的 Agent 进程记录。
type AgentProcess struct {
	mu          sync.RWMutex // 保护由 runtime 执行期间更新、控制通道同时读取的路径。
	ID          string       // 进程 ID
	Name        string       // 进程名
	State       ProcessState // 生命周期状态
	Spec        ProcessSpec  // 启动提示词和退出条件
	StartedAt   time.Time    // 启动时间
	EndedAt     time.Time    // 结束时间
	ReportPath  string       // 进程报告路径
	WorkLogPath string       // 进程工作日志路径
	cancel      context.CancelFunc
}

// SetWorkLogPath 发布运行中 worklog，供 attach 客户端读取。
func (p *AgentProcess) SetWorkLogPath(path string) {
	p.mu.Lock()
	p.WorkLogPath = path
	p.mu.Unlock()
}

func (p *AgentProcess) workLogPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.WorkLogPath
}

// ProcessRunner 是 AgentProcess 的执行层接口。
type ProcessRunner interface {
	RunProcess(ctx context.Context, proc *AgentProcess) error
}

// Agentd 管理 AgentProcess 生命周期。
// 必须通过 New 创建；cond 和内部 map 依赖 New 完成初始化。
type Agentd struct {
	mu        sync.Mutex               // 保护进程表和 nextPID
	cond      *sync.Cond               // 等待事件队列
	processes map[string]*AgentProcess // 进程表；后续实现前不得外泄可变引用
	events    []Event                  // 内存事件队列
	seen      map[string]bool          // 已入队事件 ID，用于去重
	nextPID   int                      // 本地递增进程号
}

// New 创建 Agentd。
// 参数：无。
// 调用层级：外部入口 -> New -> Run/RunProcess。
// 步骤：初始化进程表；不读取配置，不创建 runtime，不调用 LLM。
func New() *Agentd {
	s := &Agentd{processes: map[string]*AgentProcess{}, seen: map[string]bool{}, nextPID: 1}
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
func (s *Agentd) Run(ctx context.Context, runner ProcessRunner) error {
	// 1. cond 无法直接等待 context；辅助 goroutine 在取消时唤醒阻塞的 nextEvent。
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
	// 2. 主循环串行消费事件；process.start 只负责异步启动 runner，不阻塞后续事件。
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

// 必须在持有 Agentd.mu 时调用。
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

// dispatch 将 process.start 异步交给 ProcessRunner，其他事件只更新调度状态。
func (s *Agentd) dispatch(ctx context.Context, runner ProcessRunner, event Event) error {
	if event.Type == "process.start" {
		if runner == nil {
			return fmt.Errorf("process runner is required")
		}
		spec, err := processStartFromEvent(event)
		if err != nil {
			return fmt.Errorf("parse %s payload: %w", event.Type, err)
		}
		// runner 异步执行；若校验失败且尚未创建进程，补发无 ProcessID 的失败事实。
		go func() {
			proc, err := s.RunProcess(ctx, runner, spec)
			if err == nil || proc != nil {
				return
			}
			id := ""
			if event.ID != "" {
				id = event.ID + ".failed"
			}
			s.Emit(Event{ID: id, Type: "process.failed", Source: "agentd.dispatch", CreatedAt: time.Now().UTC()})
		}()
		return nil
	}
	s.applyEvent(event)
	return nil
}

// processStartFromEvent 严格解析 process.start 的启动规格。
func processStartFromEvent(event Event) (ProcessSpec, error) {
	var payload ProcessStartPayload
	if err := decodeStrict(event.Payload, &payload); err != nil {
		return ProcessSpec{}, err
	}
	return payload.ProcessSpec, nil
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

// nextEvent 通过 cond 等待事件并按 FIFO 取出；Run 的取消协程负责唤醒等待者。
func (s *Agentd) nextEvent(ctx context.Context) (Event, bool) {
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

// applyEvent 只终结 ProcessID 匹配的进程；无 ProcessID 的结束事件仅作为失败事实保留。
func (s *Agentd) applyEvent(event Event) {
	if (event.Type == "process.exited" || event.Type == "process.failed") && event.ProcessID == "" {
		return
	}
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
// 事件队列
// =============================================================================

// Emit 写入一个调度事件。
// 参数：event 是外部或内部事实；ID 可为空。
// 调用层级：控制面 / Runtime runner / 系统工具 -> Emit -> Run。
// 步骤：按非空 ID 去重 -> 补 CreatedAt -> 追加到内存事件队列。
func (s *Agentd) Emit(event Event) {
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

// =============================================================================
// 进程执行：串行创建 AgentProcess，并把执行交给外部 runner
// =============================================================================

// RunProcess 同步运行一个 AgentProcess。
// 参数：ctx 控制执行生命周期；runner 是外部执行引擎；spec 只包含 SystemPrompt/ExitCondition。
// 调用层级：Agentd 单进程调度 -> RunProcess -> runner.RunProcess。
// 步骤：创建进程记录 -> 标记 running -> 调用 runner -> 按错误标记状态 -> 投递结束事件。
func (s *Agentd) RunProcess(ctx context.Context, runner ProcessRunner, spec ProcessSpec) (*AgentProcess, error) {
	if runner == nil {
		return nil, fmt.Errorf("process runner is required")
	}
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	// 1. 在同一临界区检查单进程约束并发布 running 记录，避免并发启动穿透。
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
		ID:        id,
		Name:      id,
		State:     ProcessRunning,
		Spec:      spec,
		StartedAt: now,
		cancel:    cancel,
	}
	s.processes[id] = proc
	s.mu.Unlock()

	// 2. 锁外同步执行 runner，保证控制面仍可读取进程和请求取消。
	err := runner.RunProcess(runCtx, proc)
	// 3. 在锁内固定终态，再于锁外投递结束事件，避免 Emit 重入同一把锁。
	s.mu.Lock()
	eventType := "process.exited"
	if err != nil {
		proc.terminate(ProcessFailed)
		endedAt := proc.EndedAt
		eventType = "process.failed"
		s.mu.Unlock()
		s.Emit(Event{Type: eventType, Source: "agentd", ProcessID: proc.ID, CreatedAt: endedAt})
		return proc, err
	}
	proc.terminate(ProcessExited)
	endedAt := proc.EndedAt
	s.mu.Unlock()
	s.Emit(Event{Type: eventType, Source: "agentd", ProcessID: proc.ID, CreatedAt: endedAt})
	return proc, nil
}
