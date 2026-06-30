// systemd.go - Agent Systemd 顶层调度骨架
// 功能：定义 Agent 进程、启动提示词、退出条件、事件和 decision 的最小接口。
// 调用方：由上层创建调度器，由 internal/runtime 作为 ProcessRunner 执行真实 Agent。
// 全局变量：无。AgentSystemd 的状态应放在结构体实例内，避免多调度器互相污染。
package systemd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/lzq/5hAgent/internal/ipctypes"
)

// ProcessState 表示 AgentProcess 生命周期状态。
type ProcessState string

const (
	ProcessNew     ProcessState = "new"
	ProcessRunning ProcessState = "running"
	ProcessExited  ProcessState = "exited"
	ProcessFailed  ProcessState = "failed"
	ProcessStopped ProcessState = "stopped"
)

// DecisionAction 是 decision 允许返回的动作枚举。
type DecisionAction string

const (
	DecisionStart    DecisionAction = "start_agent"
	DecisionWait     DecisionAction = "wait"
	DecisionRetry    DecisionAction = "retry_agent"
	DecisionEscalate DecisionAction = "escalate"
	DecisionStop     DecisionAction = "stop_agent"
)

// SkillRef 是启动提示词里暴露给 Agent 的 skill 摘要。
type SkillRef struct {
	Name        string `json:"name"`        // skill 名称
	Description string `json:"description"` // skill 使用场景摘要
}

// PromptSpec 是启动 AgentProcess 时传入的完整提示词配置。
type PromptSpec struct {
	System string     `json:"system"` // system prompt
	Skills []SkillRef `json:"skills"` // 已激活 skill 的名称和描述，不含正文
}

// ExitSpec 描述 AgentProcess 的外部退出条件。
type ExitSpec struct {
	Condition string    `json:"condition,omitempty"` // 自然语言退出条件
	Deadline  time.Time `json:"deadline,omitempty"`  // 最晚退出时间
	MaxTurns  int       `json:"max_turns,omitempty"` // 最大交互轮次
}

// ProcessSpec 是启动 AgentProcess 的唯一输入。
type ProcessSpec struct {
	Prompt PromptSpec `json:"prompt"` // 启动提示词
	Exit   ExitSpec   `json:"exit"`   // 退出条件
}

// Event 是 Agent Systemd 调度循环处理的事实输入。
type Event struct {
	ID        string          `json:"id"`         // 事件 ID，用于去重
	Type      string          `json:"type"`       // 事件类型，如 timer.tick/process.exited
	Source    string          `json:"source"`     // 事件来源
	ProcessID string          `json:"process_id"` // 目标进程 ID，可为空表示广播事件
	Risk      string          `json:"risk"`       // high 表示只记录不自动执行
	Payload   json.RawMessage `json:"payload"`    // 结构化负载
	CreatedAt time.Time       `json:"created_at"` // 创建时间
}

// ProcessEventPayload 是进程结束类事件的最小负载。
type ProcessEventPayload struct {
	Error       string `json:"error,omitempty"`        // runner 返回的错误文本
	ReportPath  string `json:"report_path,omitempty"`  // 进程报告路径
	WorkLogPath string `json:"worklog_path,omitempty"` // 进程工作日志路径
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
	FileEvent   Event       `json:"file_event"`   // 触发的文件事件
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

// IPC 是 Agent Systemd 内部使用的结构化进程通信接口。
type IPC interface {
	Send(msg ipctypes.Message) error
	Recv(pid string) ([]ipctypes.Message, error)
}

// AgentProcess 是一个运行中的 Agent 进程记录。
type AgentProcess struct {
	ID           string       // 进程 ID
	Name         string       // 进程名
	State        ProcessState // 生命周期状态
	Spec         ProcessSpec  // 启动提示词和退出条件
	StartedAt    time.Time    // 启动时间
	EndedAt      time.Time    // 结束时间
	LastActiveAt time.Time    // 最近活跃时间
	LastEvent    Event        // 最近处理的事件
	SourceTask   SourceTask   // 来源任务；为空表示非 task 事件启动
	ReportPath   string       // 进程报告路径
	WorkLogPath  string       // 进程工作日志路径
	Turns        int          // 已执行轮次；由后续 runtime 回填
	cancel       context.CancelFunc
}

// Policy 是交给 decision 的硬编码调度规则摘要。
type Policy struct {
	DedupEventID  bool   `json:"dedup_event_id"` // 非空事件 ID 会去重
	SingleProcess bool   `json:"single_process"` // 当前只允许一个 running 进程
	HighRiskWait  bool   `json:"high_risk_wait"` // high risk 事件只记录，不启动进程或 decision
	MaxRetries    int    `json:"max_retries"`    // task.failed 最多触发几次 decision
	StalledAfter  string `json:"stalled_after"`  // running 进程无活跃多久后停止
}

// DecisionInput 是交给 decision 的结构化事实。
type DecisionInput struct {
	Event     Event          `json:"event"`     // 当前事件
	Processes []AgentProcess `json:"processes"` // 当前进程表快照
	Policy    Policy         `json:"policy"`    // 硬编码调度规则摘要
}

// DecisionResult 是 decision 返回的结构化判断。
type DecisionResult struct {
	Action DecisionAction `json:"action"`                 // 允许的动作枚举
	Reason string         `json:"reason,omitempty"`       // 判断理由
	Spec   ProcessSpec    `json:"process_spec,omitempty"` // 仅 start_agent/retry_agent 有效
	Target string         `json:"target_process_id"`      // stop_agent 的目标进程 ID
}

// ProcessRunner 是 AgentProcess 的执行层接口。
type ProcessRunner interface {
	RunProcess(ctx context.Context, proc *AgentProcess, ipc IPC) error
}

// DecisionCaller 是受控 LM 判断入口。
type DecisionCaller interface {
	CallDecision(ctx context.Context, input []byte) ([]byte, error)
}

// AgentSystemd 管理 AgentProcess 生命周期。
// 必须通过 New 创建；cond 和内部 map 依赖 New 完成初始化。
type AgentSystemd struct {
	mu           sync.Mutex                    // 保护进程表和 nextPID
	cond         *sync.Cond                    // 等待事件队列
	processes    map[string]*AgentProcess      // 进程表；后续实现前不得外泄可变引用
	mailbox      map[string][]ipctypes.Message // 进程短消息队列
	events       []Event                       // 内存事件队列
	seen         map[string]bool               // 已入队事件 ID，用于去重
	retries      map[string]int                // task.failed 重试计数
	maxRetry     int                           // 单个失败 key 最多触发几次 decision
	stalledAfter time.Duration                 // running 进程多久无活跃后视为 stalled
	nextPID      int                           // 本地递增进程号
}

// Option 调整 AgentSystemd 的保守调度策略。
type Option func(*AgentSystemd)

// WithMaxRetry 设置同一失败来源最多触发几次 decision retry。
func WithMaxRetry(n int) Option {
	return func(s *AgentSystemd) {
		if n >= 0 {
			s.maxRetry = n
		}
	}
}

// WithStalledAfter 设置 running 进程无活跃多久后被 timer.tick 停止；<=0 表示关闭。
func WithStalledAfter(d time.Duration) Option {
	return func(s *AgentSystemd) {
		s.stalledAfter = d
	}
}

// New 创建 AgentSystemd。
// 参数：opts 是可选调度策略，默认 maxRetry=1、stalledAfter=30m。
// 调用层级：外部入口 -> New -> Run/RunProcess。
// 步骤：初始化进程表 -> 应用 options；不读取配置，不创建 runtime，不调用 LLM。
func New(opts ...Option) *AgentSystemd {
	s := &AgentSystemd{processes: map[string]*AgentProcess{}, mailbox: map[string][]ipctypes.Message{}, seen: map[string]bool{}, retries: map[string]int{}, maxRetry: 1, stalledAfter: 30 * time.Minute, nextPID: 1}
	s.cond = sync.NewCond(&s.mu)
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// Run 启动调度循环。
// 参数：ctx 控制调度器生命周期；caller 为空时只执行硬编码调度。
// 调用层级：外部入口 -> Run -> dispatch/Decision/RunProcess。
// 步骤：接收事件 -> 套用硬编码策略 -> 必要时 decision 升级判断 -> 调度进程。
func (s *AgentSystemd) Run(ctx context.Context, runner ProcessRunner, caller DecisionCaller) error {
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
		if caller != nil && event.Type == "task.failed" && event.Risk != "high" {
			// Source 优先：同一外部来源的重复失败只算一次重试。
			key := event.Source
			if key == "" {
				key = event.ProcessID
			}
			if key == "" {
				key = event.ID
			}
			if key != "" {
				s.mu.Lock()
				if s.retries[key] >= s.maxRetry {
					s.mu.Unlock()
					continue
				}
				s.retries[key]++
				s.mu.Unlock()
			}
			decision, err := s.Decision(ctx, caller, s.decisionInput(event))
			if err != nil {
				return err
			}
			if _, err := s.applyDecision(ctx, runner, decision); err != nil {
				return err
			}
		}
	}
}

func validateSpec(spec ProcessSpec) error {
	if spec.Prompt.System == "" {
		return fmt.Errorf("prompt system is required")
	}
	if spec.Exit.Condition == "" && spec.Exit.MaxTurns <= 0 && spec.Exit.Deadline.IsZero() {
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
	if event.Risk == "high" {
		s.applyEvent(event)
		return nil
	}
	if event.Type == "process.start" || event.Type == "task.created" || event.Type == "manual.request" {
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
			payload, _ := json.Marshal(ProcessEventPayload{Error: err.Error()})
			s.Emit(Event{ID: id, Type: "process.failed", Source: "systemd.dispatch", Payload: payload, CreatedAt: time.Now().UTC()})
		}()
		return nil
	}
	s.applyEvent(event)
	return nil
}

func processStartFromEvent(event Event) (ProcessSpec, SourceTask, error) {
	switch event.Type {
	case "task.created":
		var payload TaskCreatedPayload
		if err := decodeStrict(event.Payload, &payload); err != nil {
			return ProcessSpec{}, SourceTask{}, err
		}
		return payload.ProcessSpec, SourceTask{ID: payload.TaskID, Title: payload.TaskTitle, EventID: event.ID}, nil
	default:
		var spec ProcessSpec
		if err := decodeStrict(event.Payload, &spec); err != nil {
			return ProcessSpec{}, SourceTask{}, err
		}
		return spec, SourceTask{EventID: event.ID}, nil
	}
}

func processSpecFromEvent(event Event) (ProcessSpec, error) {
	spec, _, err := processStartFromEvent(event)
	return spec, err
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *AgentSystemd) applyDecision(ctx context.Context, runner ProcessRunner, decision DecisionResult) (*AgentProcess, error) {
	switch decision.Action {
	case DecisionStart, DecisionRetry:
		return s.RunProcess(ctx, runner, decision.Spec)
	case DecisionStop:
		if decision.Target == "" {
			return nil, fmt.Errorf("stop_agent requires target_process_id")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		proc, ok := s.processes[decision.Target]
		if !ok || proc == nil {
			return nil, fmt.Errorf("process not found: %s", decision.Target)
		}
		proc.terminate(ProcessStopped)
		return proc, nil
	case DecisionWait, DecisionEscalate:
		return nil, nil
	default:
		return nil, fmt.Errorf("invalid decision action: %s", decision.Action)
	}
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
		proc.LastEvent = event
		if event.Type != "timer.tick" && !event.CreatedAt.IsZero() {
			proc.LastActiveAt = event.CreatedAt
		}
		switch event.Type {
		case "process.exited":
			proc.terminate(ProcessExited)
			s.clearRetry(event)
		case "process.failed":
			proc.terminate(ProcessFailed)
			s.clearRetry(event)
		case "process.stopped":
			proc.terminate(ProcessStopped)
			s.clearRetry(event)
		case "timer.tick":
			if shouldExit(proc, event) {
				proc.terminate(ProcessStopped)
			} else if proc.State == ProcessRunning && s.stalledAfter > 0 && !proc.LastActiveAt.IsZero() && time.Since(proc.LastActiveAt) >= s.stalledAfter {
				proc.terminate(ProcessStopped)
			}
		}
	}
}

func (s *AgentSystemd) clearRetry(event Event) {
	if event.Source != "" {
		delete(s.retries, event.Source)
	}
	if event.ProcessID != "" {
		delete(s.retries, event.ProcessID)
	}
	if event.ID != "" {
		delete(s.retries, event.ID)
	}
}

func shouldExit(proc *AgentProcess, event Event) bool {
	if proc == nil {
		return true
	}
	if proc.State == ProcessExited || proc.State == ProcessFailed || proc.State == ProcessStopped {
		return true
	}
	if event.ProcessID != "" && event.ProcessID != proc.ID {
		return false
	}
	if event.Type == "process.exited" || event.Type == "task.completed" {
		return true
	}
	if proc.Spec.Exit.MaxTurns > 0 && proc.Turns >= proc.Spec.Exit.MaxTurns {
		return true
	}
	if !proc.Spec.Exit.Deadline.IsZero() && !time.Now().Before(proc.Spec.Exit.Deadline) {
		return true
	}
	return false
}

func (s *AgentSystemd) decisionInput(event Event) DecisionInput {
	return DecisionInput{
		Event:     event,
		Processes: s.ListProcesses(),
		Policy: Policy{
			DedupEventID:  true,
			SingleProcess: true,
			HighRiskWait:  true,
			MaxRetries:    s.maxRetry,
			StalledAfter:  s.stalledAfter.String(),
		},
	}
}

// Decision 在硬编码规则无法判断时做一次受控 LM 升级。
// 参数：caller 执行唯一 LM 调用；input 是事件、进程表和 policy 摘要。
// 调用层级：Run -> Decision -> caller.CallDecision -> ParseDecision。
// 步骤：编码 JSON -> 调用一次 caller -> 解析并校验 JSON。
func (s *AgentSystemd) Decision(ctx context.Context, caller DecisionCaller, input DecisionInput) (DecisionResult, error) {
	if caller == nil {
		return DecisionResult{}, fmt.Errorf("decision caller is required")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return DecisionResult{}, fmt.Errorf("marshal decision input: %w", err)
	}
	out, err := caller.CallDecision(ctx, data)
	if err != nil {
		return DecisionResult{}, err
	}
	return ParseDecision(out)
}

// ParseDecision 解析并校验 decision JSON。
// 参数：data 是 LM 返回的 JSON 字节。
// 调用层级：Decision -> ParseDecision -> RunProcess。
// 步骤：反序列化 -> 校验 action -> 按 action 校验 ProcessSpec。
func ParseDecision(data []byte) (DecisionResult, error) {
	var result DecisionResult
	if err := decodeStrict(data, &result); err != nil {
		return DecisionResult{}, fmt.Errorf("parse decision json: %w", err)
	}
	switch result.Action {
	case DecisionStart, DecisionRetry:
		if err := validateSpec(result.Spec); err != nil {
			return DecisionResult{}, fmt.Errorf("decision process %w", err)
		}
		for _, skill := range result.Spec.Prompt.Skills {
			if skill.Name == "" {
				return DecisionResult{}, fmt.Errorf("decision skill name is required")
			}
		}
	case DecisionStop:
		if result.Target == "" {
			return DecisionResult{}, fmt.Errorf("stop_agent target_process_id is required")
		}
	case DecisionWait, DecisionEscalate:
	default:
		return DecisionResult{}, fmt.Errorf("invalid decision action: %s", result.Action)
	}
	return result, nil
}

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
	if event.ID != "" && event.Type != "timer.tick" {
		if s.seen[event.ID] {
			return
		}
		s.seen[event.ID] = true
	}
	s.events = append(s.events, event)
	s.cond.Signal()
}

// StartTimer 启动周期性 timer.tick 事件源。
// 参数：interval 是 tick 间隔；<=0 时不启动。
// 调用层级：外部入口 -> StartTimer -> Emit(timer.tick) -> Run。
// 步骤：启动 goroutine -> 按 interval 发 timer.tick -> ctx 取消时退出。
func (s *AgentSystemd) StartTimer(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				s.Emit(Event{ID: "timer.tick." + t.UTC().Format(time.RFC3339Nano), Type: "timer.tick", Source: "timer", CreatedAt: t.UTC()})
			}
		}
	}()
}

// StartSource 启动外部事件源。
// 参数：source 是 watcher/manual/timer 之外的事件生产者。
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
				return
			}
			s.Emit(event)
		}
	}()
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

// ListProcesses 返回当前进程表快照。
// 参数：无。
// 调用层级：Policy/Decision -> ListProcesses。
// 步骤：复制进程结构体列表；不返回内部 map 和指针，避免外部改写进程表。
func (s *AgentSystemd) ListProcesses() []AgentProcess {
	s.mu.Lock()
	defer s.mu.Unlock()
	processes := make([]AgentProcess, 0, len(s.processes))
	for _, proc := range s.processes {
		if proc == nil {
			continue
		}
		processes = append(processes, *proc)
	}
	return processes
}

// RunProcess 同步运行一个 AgentProcess。
// 参数：ctx 控制执行生命周期；runner 是外部执行引擎；spec 只包含 PromptSpec/ExitSpec。
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
		ID:           id,
		Name:         id,
		State:        ProcessRunning,
		Spec:         spec,
		StartedAt:    now,
		LastActiveAt: now,
		SourceTask:   source,
		cancel:       cancel,
	}
	s.processes[id] = proc
	s.mu.Unlock()

	err := runner.RunProcess(runCtx, proc, s)
	s.mu.Lock()
	eventType := "process.exited"
	// json.Marshal on ProcessEventPayload cannot fail: it only contains strings.
	payload, _ := json.Marshal(ProcessEventPayload{ReportPath: proc.ReportPath, WorkLogPath: proc.WorkLogPath})
	if err != nil {
		proc.terminate(ProcessFailed)
		endedAt := proc.EndedAt
		eventType = "process.failed"
		// json.Marshal on ProcessEventPayload cannot fail: it only contains strings.
		payload, _ = json.Marshal(ProcessEventPayload{Error: err.Error(), ReportPath: proc.ReportPath, WorkLogPath: proc.WorkLogPath})
		s.mu.Unlock()
		s.Emit(Event{Type: eventType, Source: "systemd", ProcessID: proc.ID, Payload: payload, CreatedAt: endedAt})
		return proc, err
	}
	proc.terminate(ProcessExited)
	endedAt := proc.EndedAt
	s.mu.Unlock()
	s.Emit(Event{Type: eventType, Source: "systemd", ProcessID: proc.ID, Payload: payload, CreatedAt: endedAt})
	return proc, nil
}

// Send 投递一条进程间短消息。
// 参数：msg.To 是目标进程 ID；Summary 是短文本；Artifact 是可选文件路径。
// 调用层级：后续 sys.ipc.send 工具 -> Send。
// 步骤：校验目标进程存在 -> 补 CreatedAt -> 放入目标进程 mailbox。
func (s *AgentSystemd) Send(msg ipctypes.Message) error {
	if msg.To == "" {
		return fmt.Errorf("ipc target is required")
	}
	if msg.Summary == "" && msg.Artifact == "" {
		return fmt.Errorf("ipc summary or artifact is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.processes[msg.To]; !ok {
		return fmt.Errorf("process not found: %s", msg.To)
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	s.mailbox[msg.To] = append(s.mailbox[msg.To], msg)
	// TODO: 确认 IPC 收信是否应该通过 ipc.message 事件更新活跃度；当前保持同步可见。
	if proc := s.processes[msg.To]; proc != nil {
		proc.LastActiveAt = msg.CreatedAt
	}
	return nil
}

// Recv 拉取并清空目标进程的短消息队列。
// 参数：pid 是接收方进程 ID。
// 调用层级：后续 sys.ipc.recv 工具 -> Recv。
// 步骤：校验进程存在 -> 复制消息 -> 清空 mailbox；不返回任何 context。
func (s *AgentSystemd) Recv(pid string) ([]ipctypes.Message, error) {
	if pid == "" {
		return nil, fmt.Errorf("process id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.processes[pid]; !ok {
		return nil, fmt.Errorf("process not found: %s", pid)
	}
	messages := append([]ipctypes.Message(nil), s.mailbox[pid]...)
	delete(s.mailbox, pid)
	return messages, nil
}
