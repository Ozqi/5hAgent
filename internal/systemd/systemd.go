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
	"strings"
	"sync"
	"time"

	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/skill"
	"github.com/lzq/5hAgent/internal/task"
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

// TaskSpecBuilder 把任务转换为 AgentProcess 启动规格。
type TaskSpecBuilder func(t *task.Task) (ProcessSpec, error)

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

// TaskFileEventSource 把 task.md 文件变化解析成 task.created 事件。
type TaskFileEventSource struct {
	file  *FileEventSource // 底层文件 watcher
	list  *task.TaskList   // 任务列表
	build TaskSpecBuilder  // 任务到 ProcessSpec 的转换策略
}

// StartEvent 创建启动 AgentProcess 的事件。
// 参数：eventType 可为 process.start/task.created/manual.request；id/source 用于去重和追踪。
// 调用层级：外部调度入口 -> StartEvent -> Emit -> Run。
// 步骤：编码 ProcessSpec 到 Payload；补事件类型和创建时间。
func StartEvent(eventType string, id string, source string, spec ProcessSpec) (Event, error) {
	if eventType == "" {
		eventType = "process.start"
	}
	payload, err := json.Marshal(spec)
	if err != nil {
		return Event{}, fmt.Errorf("marshal process spec: %w", err)
	}
	return Event{ID: id, Type: eventType, Source: source, Payload: payload, CreatedAt: time.Now().UTC()}, nil
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

// NewTaskFileEventSource 创建 task.md 事件源。
// 参数：list 是已有任务列表；builder 由上层决定如何把 task 转成 PromptSpec/ExitSpec。
// 调用层级：外部入口 -> NewTaskFileEventSource -> StartSource -> Run。
// 步骤：复用 FileEventSource 监听文件变化；变化后选 in_progress/pending 任务并生成 task.created。
func NewTaskFileEventSource(list *task.TaskList, interval time.Duration, builder TaskSpecBuilder) *TaskFileEventSource {
	path := ""
	if list != nil {
		path = list.Path()
	}
	return &TaskFileEventSource{file: NewFileEventSource(path, "task.changed", "task", interval), list: list, build: builder}
}

// IPCMessage 是 AgentProcess 之间的短消息。
type IPCMessage struct {
	From      string    `json:"from"`       // 发送方进程 ID
	To        string    `json:"to"`         // 接收方进程 ID
	Summary   string    `json:"summary"`    // 短消息摘要
	Artifact  string    `json:"artifact"`   // 大内容 artifact 路径，可为空
	CreatedAt time.Time `json:"created_at"` // 创建时间
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
	RunProcess(ctx context.Context, proc *AgentProcess) error
}

// DecisionCaller 是受控 LM 判断入口。
type DecisionCaller interface {
	CallDecision(ctx context.Context, input []byte) ([]byte, error)
}

// AgentSystemd 管理 AgentProcess 生命周期。
type AgentSystemd struct {
	mu           sync.Mutex               // 保护进程表和 nextPID
	cond         *sync.Cond               // 等待事件队列
	processes    map[string]*AgentProcess // 进程表；后续实现前不得外泄可变引用
	mailbox      map[string][]IPCMessage  // 进程短消息队列
	events       []Event                  // 内存事件队列
	seen         map[string]bool          // 已入队事件 ID，用于去重
	retries      map[string]int           // task.failed 重试计数
	maxRetry     int                      // 单个失败 key 最多触发几次 decision
	stalledAfter time.Duration            // running 进程多久无活跃后视为 stalled
	nextPID      int                      // 本地递增进程号
}

// New 创建 AgentSystemd。
// 参数：无。
// 调用层级：外部入口 -> New -> Run/StartProcess。
// 步骤：初始化进程表；不读取配置，不创建 runtime，不调用 LLM。
func New() *AgentSystemd {
	s := &AgentSystemd{processes: map[string]*AgentProcess{}, mailbox: map[string][]IPCMessage{}, seen: map[string]bool{}, retries: map[string]int{}, maxRetry: 1, stalledAfter: 30 * time.Minute, nextPID: 1}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// SkillRefs 从已加载 skill 快照中提取启动提示词需要的摘要。
// 参数：skills 是 skill.Manager.ListSkills() 返回的列表。
// 调用层级：Agent Systemd 启动流程 -> SkillRefs -> ProcessSpec.Prompt。
// 步骤：跳过 nil、未启用、无名 skill；只复制 Name/Description，不读取 Content。
func SkillRefs(skills []*skill.Skill) []SkillRef {
	refs := make([]SkillRef, 0, len(skills))
	for _, s := range skills {
		if s == nil || !s.Enabled || s.Name == "" {
			continue
		}
		refs = append(refs, SkillRef{Name: s.Name, Description: s.Description})
	}
	return refs
}

// Run 启动调度循环。
// 参数：ctx 控制调度器生命周期。
// 调用层级：外部入口 -> Run -> NextEvent/ApplyPolicy/Decision/Dispatch。
// 步骤：接收事件 -> 套用硬编码策略 -> 必要时 decision 升级判断 -> 调度进程。
func (s *AgentSystemd) Run(ctx context.Context, runner ProcessRunner) error {
	return s.run(ctx, runner, nil)
}

// RunWithDecision 启动带受控 decision 的调度循环。
// 参数：runner 执行进程；caller 只在硬编码规则无法判断时调用一次 LM。
// 调用层级：外部入口 -> RunWithDecision -> DispatchEvent/Decision/ApplyDecision。
// 步骤：接收事件 -> 先硬编码处理 -> task.failed 时调用 decision -> 应用 decision。
func (s *AgentSystemd) RunWithDecision(ctx context.Context, runner ProcessRunner, caller DecisionCaller) error {
	return s.run(ctx, runner, caller)
}

func (s *AgentSystemd) run(ctx context.Context, runner ProcessRunner, caller DecisionCaller) error {
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
		if _, err := s.DispatchEvent(ctx, runner, event); err != nil {
			return err
		}
		if caller != nil && event.Type == "task.failed" && event.Risk != "high" {
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
			if _, err := s.ApplyDecision(ctx, runner, decision); err != nil {
				return err
			}
		}
	}
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
	if event.ID != "" {
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

// Next 等待 task.md 变化并返回 task.created 事件。
// 参数：ctx 控制等待生命周期。
// 调用层级：StartSource -> TaskFileEventSource.Next -> FileEventSource.Next -> TaskList.ListTasksByStatus。
// 步骤：等待文件变化 -> 读取任务列表 -> 选择 in_progress/pending -> 调 builder 生成 ProcessSpec。
func (w *TaskFileEventSource) Next(ctx context.Context) (Event, error) {
	if w == nil || w.file == nil || w.list == nil {
		return Event{}, fmt.Errorf("task file event source path is required")
	}
	if w.build == nil {
		return Event{}, fmt.Errorf("task spec builder is required")
	}
	for {
		fileEvent, err := w.file.Next(ctx)
		if err != nil {
			return Event{}, err
		}
		selected := nextTask(w.list)
		if selected == nil {
			continue
		}
		spec, err := w.build(selected)
		if err != nil {
			return Event{}, err
		}
		event, err := StartEvent("task.created", "task."+selected.ID+"."+fileEvent.ID, "task", spec)
		if err != nil {
			return Event{}, err
		}
		event.Payload = mustTaskPayload(spec, selected, fileEvent)
		return event, nil
	}
}

func nextTask(list *task.TaskList) *task.Task {
	for _, status := range []task.TaskStatus{task.StatusInProgress, task.StatusPending} {
		tasks := list.ListTasksByStatus(status)
		if len(tasks) > 0 {
			return tasks[0]
		}
	}
	return nil
}

func mustTaskPayload(spec ProcessSpec, selected *task.Task, fileEvent Event) json.RawMessage {
	payload, err := json.Marshal(struct {
		ProcessSpec
		TaskID    string `json:"task_id"`
		TaskTitle string `json:"task_title"`
		FileEvent Event  `json:"file_event"`
	}{ProcessSpec: spec, TaskID: selected.ID, TaskTitle: selected.Title, FileEvent: fileEvent})
	if err != nil {
		return nil
	}
	return payload
}

// DispatchEvent 处理一个已入队事件。
// 参数：runner 只在启动类事件中需要；event.Payload 对启动类事件是 ProcessSpec JSON。
// 调用层级：Run -> DispatchEvent -> RunProcess/applyEvent。
// 步骤：启动类事件解析 ProcessSpec 并启动；其他事件走 applyEvent。
func (s *AgentSystemd) DispatchEvent(ctx context.Context, runner ProcessRunner, event Event) (*AgentProcess, error) {
	if event.Risk == "high" {
		s.applyEvent(event)
		return nil, nil
	}
	if event.Type == "process.start" || event.Type == "task.created" || event.Type == "manual.request" {
		if runner == nil {
			return nil, fmt.Errorf("process runner is required")
		}
		var spec ProcessSpec
		if err := json.Unmarshal(event.Payload, &spec); err != nil {
			return nil, fmt.Errorf("parse %s payload: %w", event.Type, err)
		}
		go func() {
			_, _ = s.RunProcess(ctx, runner, spec)
		}()
		return nil, nil
	}
	s.applyEvent(event)
	return nil, nil
}

// StartProcess 创建 AgentProcess 记录。
// 参数：spec 只能包含 PromptSpec 和 ExitSpec。
// Project/WorkDir 由调用方创建 runtime 时提供，不属于 spec，也不写入 PromptSpec。
// 调用层级：Dispatch/ApplyDecision -> StartProcess -> RuntimeEngine。
// 步骤：校验启动规格 -> 创建进程记录 -> 后续接入 runtime 执行。
func (s *AgentSystemd) StartProcess(spec ProcessSpec) (*AgentProcess, error) {
	if spec.Prompt.System == "" {
		return nil, fmt.Errorf("prompt system is required")
	}
	if spec.Exit.Condition == "" && spec.Exit.MaxTurns <= 0 && spec.Exit.Deadline.IsZero() {
		return nil, fmt.Errorf("exit condition is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("agent-%d", s.nextPID)
	s.nextPID++
	now := time.Now().UTC()
	proc := &AgentProcess{
		ID:           id,
		Name:         id,
		State:        ProcessNew,
		Spec:         spec,
		StartedAt:    now,
		LastActiveAt: now,
	}
	s.processes[id] = proc
	return proc, nil
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
// 调用层级：Agent Systemd 单进程调度 -> RunProcess -> StartProcess -> runner.RunProcess。
// 步骤：创建进程记录 -> 标记 running -> 调用 runner -> 按错误标记状态 -> 投递结束事件。
func (s *AgentSystemd) RunProcess(ctx context.Context, runner ProcessRunner, spec ProcessSpec) (*AgentProcess, error) {
	if runner == nil {
		return nil, fmt.Errorf("process runner is required")
	}
	s.mu.Lock()
	for _, existing := range s.processes {
		if existing != nil && existing.State == ProcessRunning {
			s.mu.Unlock()
			return nil, fmt.Errorf("another process is already running")
		}
	}
	if spec.Prompt.System == "" {
		s.mu.Unlock()
		return nil, fmt.Errorf("prompt system is required")
	}
	if spec.Exit.Condition == "" && spec.Exit.MaxTurns <= 0 && spec.Exit.Deadline.IsZero() {
		s.mu.Unlock()
		return nil, fmt.Errorf("exit condition is required")
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
		cancel:       cancel,
	}
	s.processes[id] = proc
	s.mu.Unlock()

	runCtx = agentctx.WithSystemRuntime(runCtx, nil, nil, proc.ID, s)
	err := runner.RunProcess(runCtx, proc)
	s.mu.Lock()
	proc.EndedAt = time.Now().UTC()
	eventType := "process.exited"
	payload, _ := json.Marshal(ProcessEventPayload{ReportPath: proc.ReportPath, WorkLogPath: proc.WorkLogPath})
	if err != nil {
		proc.State = ProcessFailed
		eventType = "process.failed"
		payload, _ = json.Marshal(ProcessEventPayload{Error: err.Error(), ReportPath: proc.ReportPath, WorkLogPath: proc.WorkLogPath})
		if proc.cancel != nil {
			proc.cancel()
			proc.cancel = nil
		}
		s.mu.Unlock()
		s.Emit(Event{Type: eventType, Source: "systemd", ProcessID: proc.ID, Payload: payload, CreatedAt: proc.EndedAt})
		return proc, err
	}
	proc.State = ProcessExited
	if proc.cancel != nil {
		proc.cancel()
		proc.cancel = nil
	}
	s.mu.Unlock()
	s.Emit(Event{Type: eventType, Source: "systemd", ProcessID: proc.ID, CreatedAt: proc.EndedAt})
	return proc, nil
}

// ShouldExit 判断进程是否满足退出条件。
// 参数：proc 是进程快照；event 是触发判断的事件。
// 调用层级：Run/Dispatch -> ShouldExit。
// 步骤：检查 MaxTurns/Deadline/完成事件；不读取 Agent 内部 context。
func (s *AgentSystemd) ShouldExit(proc *AgentProcess, event Event) bool {
	return shouldExit(proc, event)
}

// Send 投递一条进程间短消息。
// 参数：msg.To 是目标进程 ID；Summary 是短文本；Artifact 是可选文件路径。
// 调用层级：后续 sys.ipc.send 工具 -> Send。
// 步骤：校验目标进程存在 -> 补 CreatedAt -> 放入目标进程 mailbox。
func (s *AgentSystemd) Send(msg IPCMessage) error {
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
	if proc := s.processes[msg.To]; proc != nil {
		proc.LastActiveAt = msg.CreatedAt
	}
	return nil
}

// SendIPC 实现 context.IPC 的发送接口。
func (s *AgentSystemd) SendIPC(from string, to string, summary string, artifact string) error {
	return s.Send(IPCMessage{From: from, To: to, Summary: summary, Artifact: artifact})
}

// Recv 拉取并清空目标进程的短消息队列。
// 参数：pid 是接收方进程 ID。
// 调用层级：后续 sys.ipc.recv 工具 -> Recv。
// 步骤：校验进程存在 -> 复制消息 -> 清空 mailbox；不返回任何 context。
func (s *AgentSystemd) Recv(pid string) ([]IPCMessage, error) {
	if pid == "" {
		return nil, fmt.Errorf("process id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.processes[pid]; !ok {
		return nil, fmt.Errorf("process not found: %s", pid)
	}
	messages := append([]IPCMessage(nil), s.mailbox[pid]...)
	delete(s.mailbox, pid)
	return messages, nil
}

// RecvIPC 实现 context.IPC 的接收接口。
func (s *AgentSystemd) RecvIPC(pid string) ([]string, error) {
	messages, err := s.Recv(pid)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts := make([]string, 0, 3)
		if msg.From != "" {
			parts = append(parts, "from="+msg.From)
		}
		if msg.Summary != "" {
			parts = append(parts, "summary="+msg.Summary)
		}
		if msg.Artifact != "" {
			parts = append(parts, "artifact="+msg.Artifact)
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out, nil
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
			proc.State = ProcessExited
			if proc.cancel != nil {
				proc.cancel()
				proc.cancel = nil
			}
			if proc.EndedAt.IsZero() {
				proc.EndedAt = time.Now().UTC()
			}
		case "process.failed":
			proc.State = ProcessFailed
			if proc.cancel != nil {
				proc.cancel()
				proc.cancel = nil
			}
			if proc.EndedAt.IsZero() {
				proc.EndedAt = time.Now().UTC()
			}
		case "process.stopped":
			proc.State = ProcessStopped
			if proc.cancel != nil {
				proc.cancel()
				proc.cancel = nil
			}
			if proc.EndedAt.IsZero() {
				proc.EndedAt = time.Now().UTC()
			}
		case "timer.tick":
			if shouldExit(proc, event) {
				proc.State = ProcessStopped
				if proc.cancel != nil {
					proc.cancel()
					proc.cancel = nil
				}
				if proc.EndedAt.IsZero() {
					proc.EndedAt = time.Now().UTC()
				}
			} else if proc.State == ProcessRunning && s.stalledAfter > 0 && !proc.LastActiveAt.IsZero() && time.Since(proc.LastActiveAt) >= s.stalledAfter {
				proc.State = ProcessStopped
				if proc.cancel != nil {
					proc.cancel()
					proc.cancel = nil
				}
				if proc.EndedAt.IsZero() {
					proc.EndedAt = time.Now().UTC()
				}
			}
		}
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
// 步骤：补进程表快照 -> 编码 JSON -> 调用一次 caller -> 解析并校验 JSON。
func (s *AgentSystemd) Decision(ctx context.Context, caller DecisionCaller, input DecisionInput) (DecisionResult, error) {
	if caller == nil {
		return DecisionResult{}, fmt.Errorf("decision caller is required")
	}
	input.Processes = s.ListProcesses()
	input.Policy = Policy{DedupEventID: true, SingleProcess: true, MaxRetries: s.maxRetry}
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

// ApplyDecision 执行已经校验过的 decision 结果。
// 参数：runner 执行 AgentProcess；decision 是 ParseDecision 返回的结果。
// 调用层级：Run/policy -> Decision -> ApplyDecision。
// 步骤：start/retry 调 RunProcess；其他动作暂不执行。
func (s *AgentSystemd) ApplyDecision(ctx context.Context, runner ProcessRunner, decision DecisionResult) (*AgentProcess, error) {
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
		proc.State = ProcessStopped
		if proc.cancel != nil {
			proc.cancel()
			proc.cancel = nil
		}
		proc.EndedAt = time.Now().UTC()
		return proc, nil
	case DecisionWait, DecisionEscalate:
		return nil, nil
	default:
		return nil, fmt.Errorf("invalid decision action: %s", decision.Action)
	}
}

// ParseDecision 解析并校验 decision JSON。
// 参数：data 是 LM 返回的 JSON 字节。
// 调用层级：Decision -> ParseDecision -> StartProcess/RunProcess。
// 步骤：反序列化 -> 校验 action -> 按 action 校验 ProcessSpec。
func ParseDecision(data []byte) (DecisionResult, error) {
	var result DecisionResult
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return DecisionResult{}, fmt.Errorf("parse decision json: %w", err)
	}
	switch result.Action {
	case DecisionStart, DecisionRetry:
		if result.Spec.Prompt.System == "" {
			return DecisionResult{}, fmt.Errorf("decision process prompt system is required")
		}
		if result.Spec.Exit.Condition == "" && result.Spec.Exit.MaxTurns <= 0 && result.Spec.Exit.Deadline.IsZero() {
			return DecisionResult{}, fmt.Errorf("decision process exit condition is required")
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
		// 这些动作不需要 ProcessSpec。
	default:
		return DecisionResult{}, fmt.Errorf("invalid decision action: %s", result.Action)
	}
	return result, nil
}
