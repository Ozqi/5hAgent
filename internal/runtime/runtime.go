// runtime.go - 无头 Agent 运行时
// 功能：集中初始化配置、LLM、Agent、本地工具，并提供基于任务文件的无头执行入口。
// 调用方：cmd/5hagent/main.go 的 TUI 入口和 headless run 子命令。
// 全局状态：复用 logger 和 tools 包内注册表；Runtime.Close 负责释放日志句柄。
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/llm"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/systemd"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/tools"
	"github.com/lzq/5hAgent/internal/utils"
)

// =============================================================================
// Runtime 配置和运行期对象
// =============================================================================

// Options 控制运行时初始化方式。
// Debug 会提升日志级别；SessionID/ContinueLast 只影响默认 MessageCtx。
// MemoryContext 为 Agent Systemd 预留：启用后不绑定 session store，Context 随进程退出销毁。
type Options struct {
	Debug         bool
	SessionID     string
	ContinueLast  bool
	MemoryContext bool
	ProjectDir    string
	LLMFormat     string
	LLMModel      string
	ModelRef      string
	PromptBase    string
}

// Runtime 持有一次 5hAgent 进程运行所需的核心对象。
// CLI/TUI 只是 Runtime 的外壳；无头运行也复用同一套 Agent、Tool、TaskList。
type Runtime struct {
	Agent      *agent.Agent
	TaskList   *task.TaskList
	CtxManager *agentctx.Manager
	MessageCtx *agentctx.Context
	SessionID  string
	PromptDir  string
	PromptBase string
	ModelName  string
	ProjectDir string

	ToolRegistry *tools.Registry            // 当前 Runtime 独立工具注册表
	plainModel   model.ToolCallingChatModel // 未绑定工具的模型，用于 no-tool AgentProcess
	hooks        *HookManager               // 项目级 runtime hooks
}

// RunOptions 描述一次文件任务执行。
// TaskID 为空时按 in_progress -> pending 顺序选择一个任务；ReportDir 为空时写入 .5hagent/reports。
type RunOptions struct {
	TaskID        string
	ReportDir     string
	WorkLog       bool // 是否在 headless 模式输出可读工作日志
	ToolEventSink func(logger.ToolEvent)
}

// RunReport 是无头执行写入报告文件前的结构化结果。
type RunReport struct {
	Task       *task.Task
	ReportPath string
	Response   string
	Err        error
	StartedAt  time.Time
	EndedAt    time.Time
}

// RunAllReport 汇总一次连续任务执行的结果。
type RunAllReport struct {
	Reports   []*RunReport
	StartedAt time.Time
	EndedAt   time.Time
}

// Summary 返回适合 TUI slash 命令展示的短结果。
func (r *RunAllReport) Summary() string {
	if r == nil || len(r.Reports) == 0 {
		return "No pending or in_progress tasks found"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Run completed: %d task(s)\n", len(r.Reports)))
	for _, report := range r.Reports {
		if report == nil || report.Task == nil {
			continue
		}
		status := string(report.Task.Status)
		if report.Err != nil {
			status = "failed"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s - %s", report.Task.ID, report.Task.Title, status))
		if report.ReportPath != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", report.ReportPath))
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

type processReport struct {
	ProcessID     string
	Source        systemd.SourceTask
	WorkLog       string
	SystemPrompt  string
	ExitCondition string
	Response      string
	Err           error
	StartedAt     time.Time
	EndedAt       time.Time
}

// =============================================================================
// Runtime 初始化：配置、LLM、Agent、工具、MCP
// =============================================================================

// New 初始化一个可交互或无头复用的 Runtime。
// 步骤：加载配置 -> 初始化日志 -> 打开任务文件和 session store -> 创建 LLM/Agent -> 注册本地工具。
// 副作用：创建 ~/.5hAgent、项目 .5hagent、日志文件；启动阶段不启动 MCP stdio 子进程。
func New(ctx context.Context, opts Options) (*Runtime, error) {
	appConfig, err := utils.LoadConfigWithOptions(utils.LoadConfigOptions{
		LLMFormat: opts.LLMFormat,
		LLMModel:  opts.LLMModel,
		ModelRef:  opts.ModelRef,
	})
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	if opts.Debug {
		appConfig.Agent.Debug = true
		logger.SetLevel(logger.DEBUG)
	}
	logFile, err := logger.InitLog()
	if err != nil {
		return nil, fmt.Errorf("init log: %w", err)
	}
	logger.InfoTag("SYS", "Log initialized: %s", logFile)

	projectRoot, err := projectRoot(opts.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("get project root: %w", err)
	}
	projectDataDir := projectDataDir(projectRoot)
	list, err := task.NewTaskList(filepath.Join(projectDataDir, "task.md"))
	if err != nil {
		return nil, fmt.Errorf("initialize task list: %w", err)
	}

	configDir, err := utils.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config directory: %w", err)
	}
	sessionDir := filepath.Join(configDir, "sessions")
	ctxManager := agentctx.NewManager(sessionDir)
	if opts.MemoryContext {
		ctxManager = agentctx.NewMemoryManagerWithStore(sessionDir)
	}
	messageCtx, sessionID, err := openMessageCtx(ctxManager, opts.SessionID, opts.ContinueLast)
	if err != nil {
		return nil, err
	}

	llmConfig := &llm.Config{
		Provider:             appConfig.LLM.Provider,
		APIKey:               appConfig.LLM.APIKey,
		BaseURL:              appConfig.LLM.BaseURL,
		Model:                appConfig.LLM.Model,
		MaxTokens:            appConfig.LLM.MaxTokens,
		ThinkingBudgetTokens: appConfig.LLM.ThinkingBudgetTokens,
	}
	client, err := llm.NewClient(ctx, llmConfig)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	promptDir := filepath.Join(configDir, "prompt")
	promptProvider := appConfig.LLM.Provider
	if appConfig.LLM.Supplier != "" {
		promptProvider = appConfig.LLM.Supplier
	}
	systemPrompt, err := utils.LoadSystemPromptBase(promptDir, opts.PromptBase, promptProvider, appConfig.LLM.Model)
	if err != nil {
		return nil, fmt.Errorf("load system prompt: %w", err)
	}

	ag, err := agent.NewAgent(nil, nil, &agent.Config{
		Name:                appConfig.Agent.Name,
		MaxTotalTokens:      appConfig.Agent.MaxTotalTokens,
		RepeatToolLimit:     appConfig.Agent.RepeatToolLimit,
		ContextAutoCompress: appConfig.Agent.ContextAutoCompress,
		Debug:               appConfig.Agent.Debug,
		DisableStream:       !appConfig.LLM.Stream,
		SystemPrompt:        systemPrompt,
		PromptDir:           promptDir,
		ProjectDataDir:      projectDataDir,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	ag.SetCtxManager(ctxManager)

	toolRegistry := tools.NewRegistry()
	toolRegistry.SetWorkspaceRoot(projectRoot)
	if err := toolRegistry.Init(list, ag.GetSkillManager()); err != nil {
		return nil, fmt.Errorf("init tools: %w", err)
	}
	toolRegistry.RegisterContextTool(client.GetModel(), promptDir)

	modelWithTools, err := bindTools(ctx, client.GetModel(), toolRegistry)
	if err != nil {
		return nil, fmt.Errorf("bind tools: %w", err)
	}
	ag.SetModel(modelWithTools)
	ag.SetTools(toolRegistry.All())

	return &Runtime{
		Agent:        ag,
		TaskList:     list,
		CtxManager:   ctxManager,
		MessageCtx:   messageCtx,
		SessionID:    sessionID,
		PromptDir:    promptDir,
		PromptBase:   promptBase(opts.PromptBase),
		ModelName:    llmConfig.Model,
		ProjectDir:   projectRoot,
		ToolRegistry: toolRegistry,
		plainModel:   client.GetModel(),
		hooks:        loadHookManager(projectRoot, sessionID),
	}, nil
}

// SwitchModel 在当前 Runtime 内切换 provider/model，并重新绑定当前工具集合。
// 参数：modelRef 使用 provider/model 格式，例如 mira/gpt-5.4。
// 边界：不改写 .env，不重写当前会话已有 system prompt；新会话会使用新模型 prefix。
func (r *Runtime) SwitchModel(ctx context.Context, modelRef string) (string, error) {
	appConfig, err := utils.LoadConfigWithOptions(utils.LoadConfigOptions{ModelRef: modelRef})
	if err != nil {
		return "", fmt.Errorf("load model config: %w", err)
	}
	llmConfig := &llm.Config{
		Provider:             appConfig.LLM.Provider,
		APIKey:               appConfig.LLM.APIKey,
		BaseURL:              appConfig.LLM.BaseURL,
		Model:                appConfig.LLM.Model,
		MaxTokens:            appConfig.LLM.MaxTokens,
		ThinkingBudgetTokens: appConfig.LLM.ThinkingBudgetTokens,
	}
	client, err := llm.NewClient(ctx, llmConfig)
	if err != nil {
		return "", fmt.Errorf("create LLM client: %w", err)
	}
	promptProvider := appConfig.LLM.Provider
	if appConfig.LLM.Supplier != "" {
		promptProvider = appConfig.LLM.Supplier
	}
	systemPrompt, err := utils.LoadSystemPromptBase(r.PromptDir, r.PromptBase, promptProvider, appConfig.LLM.Model)
	if err != nil {
		return "", fmt.Errorf("load system prompt: %w", err)
	}
	r.ToolRegistry.ReplaceContextTool(client.GetModel(), r.PromptDir)
	modelWithTools, err := bindTools(ctx, client.GetModel(), r.ToolRegistry)
	if err != nil {
		return "", fmt.Errorf("bind tools: %w", err)
	}
	r.Agent.SetModel(modelWithTools)
	r.Agent.SetTools(r.ToolRegistry.All())
	r.Agent.SetSystemPrompt(systemPrompt)
	r.ModelName = llmConfig.Model
	r.plainModel = client.GetModel()
	return r.ModelName, nil
}

func (r *Runtime) handleToolEvent(event logger.ToolEvent, taskID string, processID string) {
	if r == nil {
		return
	}
	r.recordToolFailure(event, taskID, processID)
	if r.hooks != nil {
		r.hooks.Run(event)
	}
}

func bindTools(ctx context.Context, m model.ToolCallingChatModel, registry *tools.Registry) (model.ToolCallingChatModel, error) {
	toolInfos, err := registry.ToolInfos(ctx)
	if err != nil {
		return nil, err
	}
	return m.WithTools(toolInfos)
}

// NewInMemory 初始化不绑定 session store 的 Runtime。
// 参数：ctx 控制初始化生命周期；opts 只保留 Debug 等非 session 选项。
// 调用层级：Agent Systemd -> NewInMemory -> New。
// 步骤：强制 MemoryContext=true，忽略 SessionID/ContinueLast，复用 New 的其余初始化逻辑。
func NewInMemory(ctx context.Context, opts Options) (*Runtime, error) {
	opts.MemoryContext = true
	opts.SessionID = ""
	opts.ContinueLast = false
	return New(ctx, opts)
}

// =============================================================================
// Agent Systemd 适配：Runtime 作为 ProcessRunner
// =============================================================================

// RunProcess 让 Runtime 作为 Agent Systemd 的同步执行 runner。
// 参数：proc 只读取 SystemPrompt/ExitCondition；Project/WorkDir 仍由外层启动 runtime 时决定。
// 调用层级：systemd.AgentSystemd.RunProcess -> Runtime.RunProcess -> Agent.RunStream。
// 步骤：创建内存 context -> 注入 ProcessSpec.Prompt -> 调用 Agent.RunStream -> 写进程 report/worklog。
func (r *Runtime) RunProcess(ctx context.Context, proc *systemd.AgentProcess) error {
	if proc == nil {
		return fmt.Errorf("process is nil")
	}
	started := time.Now().UTC()
	logTask := processLogTask(proc)
	if proc.SourceTask.ID != "" {
		fmt.Fprintf(os.Stdout, "process start: %s task=%s report=pending\n", proc.ID, proc.SourceTask.ID)
	} else {
		fmt.Fprintf(os.Stdout, "process start: %s report=pending\n", proc.ID)
	}
	messageCtx, err := r.CtxManager.CreateContext("")
	if err != nil {
		return fmt.Errorf("create process context: %w", err)
	}
	if proc.Spec.SystemPrompt != "" {
		if err := r.CtxManager.AddMessage(messageCtx, &schema.Message{Role: schema.System, Content: proc.Spec.SystemPrompt}); err != nil {
			return fmt.Errorf("add process system prompt: %w", err)
		}
	}
	dataDir := projectDataDir(r.ProjectDir)
	workLog := newHeadlessWorkLog(false, dataDir, proc.ID, started)
	workLog.useGlobalSink = false
	workLog.Start(logTask)
	prevSink := r.Agent.SetToolEventSink(func(event logger.ToolEvent) {
		workLog.printToolEvent(event)
		r.handleToolEvent(event, proc.SourceTask.ID, proc.ID)
	})
	defer r.Agent.SetToolEventSink(prevSink)
	input := fmt.Sprintf(`你正在以 Agent Systemd 进程模式运行。

Exit Condition:
%s

请在当前进程上下文内完成任务。上下文默认只存在于内存；如需持久化，必须显式调用系统级持久化工具。`, proc.Spec.ExitCondition)
	restoreModel := r.useProcessModel(proc)
	defer restoreModel()
	response, runErr := r.Agent.RunStreamWithOptions(ctx, messageCtx, input, workLog.OnToken, r.processModelOptions(proc), workLog.OnReasoning)
	workLog.End(runErr)
	proc.WorkLogPath = workLog.path
	if proc.SourceTask.ID != "" {
		finalStatus := task.StatusCompleted
		if runErr != nil {
			finalStatus = task.StatusFailed
		}
		if err := r.TaskList.UpdateTaskStatus(proc.SourceTask.ID, finalStatus); err != nil && runErr == nil {
			runErr = fmt.Errorf("mark source task %s: %w", finalStatus, err)
		} else if err != nil {
			logger.ErrorTag("TASK", "mark source task %s: %v", finalStatus, err)
		}
	}
	report := &processReport{ProcessID: proc.ID, Source: proc.SourceTask, WorkLog: proc.WorkLogPath, SystemPrompt: proc.Spec.SystemPrompt, ExitCondition: proc.Spec.ExitCondition, Response: response, Err: runErr, StartedAt: started, EndedAt: time.Now().UTC()}
	path, writeErr := r.writeProcessReport("", report)
	proc.ReportPath = path
	if writeErr != nil {
		return writeErr
	}
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	if proc.SourceTask.ID != "" {
		fmt.Fprintf(os.Stdout, "process %s: %s task=%s report=%s\n", status, proc.ID, proc.SourceTask.ID, proc.ReportPath)
	} else {
		fmt.Fprintf(os.Stdout, "process %s: %s report=%s\n", status, proc.ID, proc.ReportPath)
	}
	return runErr
}

// Close 释放 Runtime 启动的外部资源。
func (r *Runtime) Close() error {
	logger.CloseLog()
	return nil
}

// =============================================================================
// Headless task 执行：直接消费 .5hagent/task.md
// =============================================================================

// RunTaskOnce 从任务文件选取一个任务，调用 Agent 执行，并写入 Markdown 报告。
// 成功时任务标记为 completed；失败时标记为 failed，报告仍会落盘供下一轮恢复。
func (r *Runtime) RunTaskOnce(ctx context.Context, opts RunOptions) (*RunReport, error) {
	started := time.Now().UTC()
	selected, err := r.selectTask(opts.TaskID)
	if err != nil {
		return nil, err
	}
	if selected.Status == task.StatusPending {
		if err := r.TaskList.UpdateTaskStatus(selected.ID, task.StatusInProgress); err != nil {
			return nil, fmt.Errorf("mark task in_progress: %w", err)
		}
		selected, _ = r.TaskList.GetTask(selected.ID)
	}

	messageCtx, err := r.CtxManager.CreateContext("")
	if err != nil {
		return nil, fmt.Errorf("create task session: %w", err)
	}
	input := taskPrompt(r.TaskList.Path(), selected)
	projectDataDir := projectDataDir(r.ProjectDir)
	workLog := newHeadlessWorkLog(opts.WorkLog, projectDataDir, r.Agent.Name(), started)
	workLog.useGlobalSink = false
	workLog.Start(selected)
	prevSink := r.Agent.SetToolEventSink(func(event logger.ToolEvent) {
		workLog.printToolEvent(event)
		r.handleToolEvent(event, selected.ID, "")
		if opts.ToolEventSink != nil {
			opts.ToolEventSink(event)
		}
	})
	defer r.Agent.SetToolEventSink(prevSink)
	defer workLog.Stop()
	response, runErr := r.Agent.RunStream(ctx, messageCtx, input, workLog.OnToken, workLog.OnReasoning)
	workLog.End(runErr)

	finalStatus := task.StatusCompleted
	if runErr != nil {
		finalStatus = task.StatusFailed
	}
	if err := r.TaskList.UpdateTaskStatus(selected.ID, finalStatus); err != nil && runErr == nil {
		runErr = fmt.Errorf("mark task %s: %w", finalStatus, err)
	} else if err != nil {
		logger.ErrorTag("TASK", "mark task %s: %v", finalStatus, err)
	}
	updated, _ := r.TaskList.GetTask(selected.ID)
	if updated != nil {
		selected = updated
	}

	report := &RunReport{Task: selected, Response: response, Err: runErr, StartedAt: started, EndedAt: time.Now().UTC()}
	path, writeErr := r.writeReport(opts.ReportDir, report)
	report.ReportPath = path
	if writeErr != nil {
		return report, writeErr
	}
	return report, runErr
}

// RunTasksUntilDone 连续执行 task.md 中的 in_progress/pending 任务，直到没有可运行任务。
// 调用层级：TUI /run -> Runtime.RunTasksUntilDone -> Runtime.RunTaskOnce -> Agent.RunStream。
// 步骤：每轮选择一个任务执行；ReAct 退出后重新读取 task.md；遇到任务执行失败仍继续处理后续任务。
func (r *Runtime) RunTasksUntilDone(ctx context.Context, opts RunOptions) (*RunAllReport, error) {
	if opts.TaskID != "" {
		report, err := r.RunTaskOnce(ctx, opts)
		result := &RunAllReport{StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC()}
		if report != nil {
			result.Reports = append(result.Reports, report)
			result.StartedAt = report.StartedAt
			result.EndedAt = report.EndedAt
		}
		return result, err
	}

	result := &RunAllReport{StartedAt: time.Now().UTC()}
	var errs []string
	for {
		if err := ctx.Err(); err != nil {
			result.EndedAt = time.Now().UTC()
			return result, err
		}
		if _, err := r.selectTask(""); err != nil {
			result.EndedAt = time.Now().UTC()
			if len(errs) > 0 {
				return result, fmt.Errorf("%s", strings.Join(errs, "; "))
			}
			return result, nil
		}
		report, err := r.RunTaskOnce(ctx, opts)
		if report != nil {
			result.Reports = append(result.Reports, report)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
}

// TaskProcessSpec 把文件任务转换成 AgentProcess 启动规格。
// 参数：t 是 .5hagent/task.md 中的任务。
// 调用层级：daemon/event source -> TaskProcessSpec -> AgentSystemd.RunProcess。
// 步骤：复用 taskPrompt 生成进程 system prompt；退出条件要求写出报告或明确 blocker。
func (r *Runtime) TaskProcessSpec(t *task.Task) (systemd.ProcessSpec, error) {
	if t == nil {
		return systemd.ProcessSpec{}, fmt.Errorf("task is nil")
	}
	return systemd.ProcessSpec{
		SystemPrompt:  taskPrompt(r.TaskList.Path(), t),
		ExitCondition: "完成任务并写出可审计结果；如果遇到不可恢复阻塞，说明 blocker 后退出。",
	}, nil
}

// EmitCurrentTask 把当前 in_progress/pending 任务作为一次 task.created 事件投递。
// 参数：sys 是目标调度器。
// 调用层级：daemon 启动 -> EmitCurrentTask -> AgentSystemd.Emit。
// 步骤：选择当前任务 -> 生成 ProcessSpec -> 编码 TaskCreatedPayload -> Emit。
func (r *Runtime) EmitCurrentTask(sys *systemd.AgentSystemd) error {
	if sys == nil {
		return fmt.Errorf("systemd is nil")
	}
	selected, err := r.selectTask("")
	if err != nil {
		return err
	}
	spec, err := r.TaskProcessSpec(selected)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(systemd.TaskCreatedPayload{
		ProcessSpec: spec,
		TaskID:      selected.ID,
		TaskTitle:   selected.Title,
	})
	if err != nil {
		return fmt.Errorf("marshal initial task event: %w", err)
	}
	sys.Emit(systemd.Event{
		ID:        "task." + selected.ID + ".initial",
		Type:      "task.created",
		Source:    "task",
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	})
	return nil
}

// =============================================================================
// Runtime 内部 helper：session、MCP、task 选择和 prompt
// =============================================================================

func openMessageCtx(manager *agentctx.Manager, sessionID string, continueLast bool) (*agentctx.Context, string, error) {
	if sessionID != "" {
		ctx, err := manager.CreateContext(sessionID)
		return ctx, sessionID, err
	}
	if continueLast {
		latest, err := manager.GetLatestSessionID()
		if err != nil {
			return nil, "", fmt.Errorf("get latest session: %w", err)
		}
		if latest != "" {
			ctx, err := manager.CreateContext(latest)
			return ctx, latest, err
		}
	}
	ctx, err := manager.CreateContext("")
	if err != nil {
		return nil, "", err
	}
	return ctx, manager.GetSessionID(ctx), nil
}

func (r *Runtime) selectTask(id string) (*task.Task, error) {
	if id != "" {
		return r.TaskList.GetTask(id)
	}
	for _, status := range []task.TaskStatus{task.StatusInProgress, task.StatusPending} {
		tasks := r.TaskList.ListTasksByStatus(status)
		if len(tasks) > 0 {
			return tasks[0], nil
		}
	}
	return nil, fmt.Errorf("no pending or in_progress task found in %s", r.TaskList.Path())
}

func taskPrompt(taskPath string, t *task.Task) string {
	return fmt.Sprintf(`你正在以无头 runtime 模式执行文件任务。
任务真源：%s
任务 ID：%s
标题：%s
状态：%s

任务描述：
%s

要求：
1. 先判断任务是否需要工具；如果任务明确禁止工具，不得调用任何工具。
2. 如果任务明确指定某个工具名，必须按该工具名调用，不要用其他工具替代。
3. 如果调用工具后发现任务目标仍未达成，必须继续调用工具推进，直到目标完成或遇到明确阻塞。
4. 不要只做计划或询问确认；无头任务应自行推进到可验证结果。
5. 工具失败不是完成信号；必须读取错误、修正参数并重试，不能在失败后声称成功。
6. 如果任务要求写文件或生成报告，必须先写入，再读取或检查目标文件存在，验证成功后才能完成。
7. 如果任务给出绝对路径，必须使用该路径，不要把不同项目中的同名文件混用。
8. 最终回复前必须做内部验收清单：逐项核对任务描述中的文件、路径、小节标题、状态数量、命令和验证动作；缺任何一项都继续执行或修正。
9. 长任务执行中不要输出阶段总结、进度说明或“下一步计划”来代替行动；只要还有未完成步骤，下一轮必须继续发起工具调用。
10. 读取大文件或某函数附近内容时，先用 grep 定位行号，再用 read_file 的 offset/limit 读取小范围。
11. 如需修改代码，保持极简 baseline，优先复用已有模块。
12. 完成后给出可写入执行报告的简短结果、证据和后续建议。`, taskPath, t.ID, t.Title, t.Status, t.Description)
}

// =============================================================================
// 报告和 worklog 辅助：文件命名、渲染、路径清理
// =============================================================================

func (r *Runtime) writeReport(dir string, report *RunReport) (string, error) {
	if dir == "" {
		dir = filepath.Join(projectDataDir(r.ProjectDir), "reports")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}
	path := filepath.Join(dir, safeName(report.Task.ID)+".md")
	if err := os.WriteFile(path, []byte(renderReport(report)), 0o644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return path, nil
}

func (r *Runtime) writeProcessReport(dir string, report *processReport) (string, error) {
	if dir == "" {
		dir = filepath.Join(projectDataDir(r.ProjectDir), "reports")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create process report dir: %w", err)
	}
	name := processReportName(report)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(renderProcessReport(report)), 0o644); err != nil {
		return "", fmt.Errorf("write process report: %w", err)
	}
	return path, nil
}

func processLogTask(proc *systemd.AgentProcess) *task.Task {
	if proc == nil {
		return nil
	}
	id := proc.ID
	title := proc.Name
	if proc.SourceTask.ID != "" {
		id = proc.SourceTask.ID
		title = proc.SourceTask.Title
	}
	if title == "" {
		title = id
	}
	return &task.Task{ID: id, Title: title, Description: proc.Spec.ExitCondition}
}

func (r *Runtime) useProcessModel(proc *systemd.AgentProcess) func() {
	if r == nil || r.Agent == nil || r.plainModel == nil || !r.processForbidsTools(proc) {
		return func() {}
	}
	old := r.Agent.GetModel()
	r.Agent.SetModel(r.plainModel)
	return func() { r.Agent.SetModel(old) }
}

func (r *Runtime) processForbidsTools(proc *systemd.AgentProcess) bool {
	if r == nil || proc == nil || proc.SourceTask.ID == "" || r.TaskList == nil {
		return false
	}
	t, err := r.TaskList.GetTask(proc.SourceTask.ID)
	if err != nil {
		return false
	}
	return taskForbidsTools(t)
}

func (r *Runtime) processModelOptions(proc *systemd.AgentProcess) []model.Option {
	if r.processForbidsTools(proc) {
		return []model.Option{model.WithToolChoice(schema.ToolChoiceForbidden)}
	}
	return nil
}

func taskForbidsTools(t *task.Task) bool {
	if t == nil {
		return false
	}
	desc := strings.ToLower(t.Description)
	return strings.Contains(desc, "不要调用工具") || strings.Contains(desc, "严禁调用任何工具")
}

func processReportName(report *processReport) string {
	parts := []string{safeName(report.ProcessID)}
	if report != nil && report.Source.ID != "" {
		parts = []string{safeName(report.Source.ID), safeName(report.ProcessID)}
	}
	ts := time.Now().UTC().Format("20060102-150405.000000000")
	parts = append(parts, ts)
	return strings.Join(parts, ".") + ".md"
}

func projectDataDir(projectDir string) string {
	return filepath.Join(projectDir, ".5hagent")
}

func promptBase(base string) string {
	if strings.TrimSpace(base) == "" {
		return "main"
	}
	return base
}

func projectRoot(projectDir string) (string, error) {
	if projectDir != "" {
		return filepath.Abs(projectDir)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return cwd, nil
}

func renderReport(report *RunReport) string {
	status := "completed"
	errText := ""
	if report.Err != nil {
		status = "failed"
		errText = "\n## Error\n\n```text\n" + report.Err.Error() + "\n```\n"
	}
	return fmt.Sprintf(`# Task Report: %s

- task: %s
- status: %s
- started_at: %s
- ended_at: %s

## Task

%s

## Agent Output

%s
%s`, report.Task.Title, report.Task.ID, status, report.StartedAt.Format(time.RFC3339), report.EndedAt.Format(time.RFC3339), report.Task.Description, strings.TrimSpace(report.Response), errText)
}

func renderProcessReport(report *processReport) string {
	status := "completed"
	errText := ""
	if report.Err != nil {
		status = "failed"
		errText = "\n## Error\n\n```text\n" + report.Err.Error() + "\n```\n"
	}
	taskID := report.Source.ID
	taskTitle := report.Source.Title
	eventID := report.Source.EventID
	return fmt.Sprintf(`# Agent Process Report

- process: %s
- task: %s
- task_title: %s
- source_event: %s
- status: %s
- started_at: %s
- ended_at: %s
- worklog: %s

## Exit Condition

%s

## Agent Output

%s
%s`, report.ProcessID, taskID, taskTitle, eventID, status, report.StartedAt.Format(time.RFC3339), report.EndedAt.Format(time.RFC3339), report.WorkLog, report.ExitCondition, strings.TrimSpace(report.Response), errText)
}

func safeName(raw string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-")
	name := strings.Trim(replacer.Replace(raw), ".-")
	if name == "" {
		return "task"
	}
	return name
}
