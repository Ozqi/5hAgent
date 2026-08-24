// Package runtime 负责装配 Agent 运行时、模型、工具、上下文、daemon 会话和运行日志。
package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ozqi/walle/internal/agent"
	agentctx "github.com/Ozqi/walle/internal/context"
	"github.com/Ozqi/walle/internal/llm"
	"github.com/Ozqi/walle/internal/logger"
	"github.com/Ozqi/walle/internal/systemd"
	"github.com/Ozqi/walle/internal/toolevent"
	"github.com/Ozqi/walle/internal/tools"
	"github.com/Ozqi/walle/internal/utils"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// =============================================================================
// Runtime 配置和运行期对象
// =============================================================================

// Options 控制运行时初始化方式。
// Debug 会提升日志级别；SessionID/ContinueLast 只影响默认 MessageCtx。
type Options struct {
	Debug        bool
	SessionID    string
	ContinueLast bool
	ProjectDir   string
	LLMFormat    string
	LLMModel     string
	ModelRef     string
	PromptBase   string
}

// Runtime 持有一次 walle 进程运行所需的核心对象。
// CLI/TUI 只是 Runtime 的外壳；无头进程也复用同一套 Agent 和工具。
type Runtime struct {
	Agent      *agent.Agent
	CtxManager *agentctx.Manager
	MessageCtx *agentctx.Context
	SessionID  string
	PromptDir  string
	PromptBase string
	ModelName  string
	ModelRef   string
	ProjectDir string

	ToolRegistry *tools.Registry // 当前 Runtime 独立工具注册表
	hooks        *HookManager    // 项目级 runtime hooks
	modelMu      sync.Mutex      // 串行化 attached 客户端的模型切换
}

type processReport struct {
	ProcessID     string
	WorkLog       string
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
// 步骤：加载配置 -> 初始化日志 -> 打开 session store -> 创建 LLM/Agent -> 注册本地工具。
// 副作用：创建 ~/.walle、项目 .walle、日志文件；启动阶段不启动 MCP stdio 子进程。
func New(ctx context.Context, opts Options) (*Runtime, error) {
	// 1. 加载进程级配置和日志；此阶段可能创建用户配置目录与日志文件。
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

	// 2. 确定项目数据边界并打开消息上下文。
	projectRoot, err := projectRoot(opts.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("get project root: %w", err)
	}
	projectDataDir := projectDataDir(projectRoot)

	configDir, err := utils.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config directory: %w", err)
	}
	sessionDir := filepath.Join(configDir, "sessions")
	ctxManager := agentctx.NewManager(sessionDir)
	messageCtx, sessionID, err := openMessageCtx(ctxManager, opts.SessionID, opts.ContinueLast)
	if err != nil {
		return nil, err
	}

	// 3. 创建未绑定工具的模型，并加载与 provider/model 对应的 system prompt。
	llmConfig := &llm.Config{
		Supplier:             appConfig.LLM.Supplier,
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

	// 4. 创建共享 Agent，再注册 Runtime 私有工具表；启动阶段不会连接 MCP 进程。
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
	ag.SetDebugModel(llmConfig.Supplier+"/"+llmConfig.Model, llmConfig.APIKey)

	toolRegistry := tools.NewRegistry()
	toolRegistry.SetWorkspaceRoot(projectRoot)
	if err := toolRegistry.Init(ag.GetSkillManager()); err != nil {
		return nil, fmt.Errorf("init tools: %w", err)
	}
	toolRegistry.RegisterContextTool(client.GetModel(), promptDir)

	modelWithTools, err := bindTools(ctx, client.GetModel(), toolRegistry)
	if err != nil {
		return nil, fmt.Errorf("bind tools: %w", err)
	}
	ag.SetModel(modelWithTools)
	ag.SetTools(toolRegistry.All())
	ag.SetContextWindow(client.ContextWindow(ctx))

	// 5. 保存 Runtime 装配结果；运行进程发现统一由 supervisor 控制面提供。
	promptBase := opts.PromptBase
	if strings.TrimSpace(promptBase) == "" {
		promptBase = "main"
	}
	rt := &Runtime{
		Agent:        ag,
		CtxManager:   ctxManager,
		MessageCtx:   messageCtx,
		SessionID:    sessionID,
		PromptDir:    promptDir,
		PromptBase:   promptBase,
		ModelName:    llmConfig.Model,
		ModelRef:     llmConfig.Supplier + "/" + llmConfig.Model,
		ProjectDir:   projectRoot,
		ToolRegistry: toolRegistry,
		hooks:        loadHookManager(projectRoot, sessionID),
	}
	return rt, nil
}

// SwitchModel 在当前 Runtime 内切换 provider/model，并重新绑定当前工具集合。
// 参数：modelRef 使用 provider/model 格式，例如 mira/gpt-5.4。
// 边界：不改写 .env，不重写当前会话已有 system prompt；新会话会使用新模型 prefix。
func (r *Runtime) SwitchModel(ctx context.Context, modelRef string) (string, error) {
	// 1. 串行执行模型切换；该锁不覆盖正在运行的 Agent.RunStream，调用方应只在会话空闲时切换。
	r.modelMu.Lock()
	defer r.modelMu.Unlock()
	// 2. 从目标 provider 配置创建模型，并加载对应的 system prompt。
	appConfig, err := utils.LoadConfigWithOptions(modelLoadOptions(modelRef))
	if err != nil {
		return "", fmt.Errorf("load model config: %w", err)
	}
	llmConfig := &llm.Config{
		Supplier:             appConfig.LLM.Supplier,
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
	// 3. context 工具持有模型引用，必须先替换它，再把完整工具集合绑定到新模型。
	r.ToolRegistry.ReplaceContextTool(client.GetModel(), r.PromptDir)
	modelWithTools, err := bindTools(ctx, client.GetModel(), r.ToolRegistry)
	if err != nil {
		return "", fmt.Errorf("bind tools: %w", err)
	}
	// 4. 原子切换 Agent 后续调用使用的模型、工具和运行时展示状态；旧消息内容保持不变。
	modelRef = llmConfig.Supplier + "/" + llmConfig.Model
	r.Agent.SetModel(modelWithTools)
	r.Agent.SetTools(r.ToolRegistry.All())
	r.Agent.SetSystemPrompt(systemPrompt)
	r.Agent.SetContextWindow(client.ContextWindow(ctx))
	r.Agent.SetDebugModel(llmConfig.Supplier+"/"+llmConfig.Model, llmConfig.APIKey)
	r.ModelName = llmConfig.Model
	r.ModelRef = modelRef
	return r.ModelRef, nil
}

func (r *Runtime) handleToolEvent(event toolevent.ToolEvent, processID string) {
	if r == nil {
		return
	}
	r.recordToolFailure(event, processID)
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

// =============================================================================
// Agent Systemd 适配：Runtime 作为 ProcessRunner
// =============================================================================

// RunProcess 让 Runtime 作为 Agent Systemd 的同步执行 runner。
// 参数：读取 proc 的身份和 ProcessSpec，写回 WorkLogPath、ReportPath；Project/WorkDir 由外层决定。
// 调用层级：systemd.AgentSystemd.RunProcess -> Runtime.RunProcess -> Agent.RunStream。
// 步骤：创建内存 context -> 注入 ProcessSpec.Prompt -> 调用 Agent.RunStream -> 写进程 report/worklog。
// 边界：process.exited/process.failed 由 AgentSystemd 发布，Runtime 只执行并返回结果。
func (r *Runtime) RunProcess(ctx context.Context, proc *systemd.AgentProcess) error {
	if proc == nil {
		return fmt.Errorf("process is nil")
	}
	started := time.Now().UTC()
	fmt.Fprintf(os.Stdout, "process start: %s report=pending\n", proc.ID)
	// 1. 为进程创建独立上下文，并把进程 prompt 作为首条 system 消息注入。
	messageCtx, err := r.CtxManager.CreateContext("")
	if err != nil {
		return fmt.Errorf("create process context: %w", err)
	}
	if proc.Spec.SystemPrompt != "" {
		if err := r.CtxManager.AddMessage(messageCtx, &schema.Message{Role: schema.System, Content: proc.Spec.SystemPrompt}); err != nil {
			return fmt.Errorf("add process system prompt: %w", err)
		}
	}
	// 2. 建立独立 worklog 和工具事件 sink；defer 保证恢复共享 Agent 的旧 sink。
	dataDir := projectDataDir(r.ProjectDir)
	workLog := newProcessWorkLog(false, dataDir, proc.ID, started)
	workLog.Start(proc.ID, proc.Name)
	proc.SetWorkLogPath(workLog.path)
	prevSink := r.Agent.SetToolEventSink(func(event toolevent.ToolEvent) {
		workLog.printToolEvent(event)
		r.handleToolEvent(event, proc.ID)
	})
	defer r.Agent.SetToolEventSink(prevSink)
	input := fmt.Sprintf(`你正在以 Agent Systemd 进程模式运行。

Exit Condition:
%s

请在当前进程上下文内完成任务。上下文由 Runtime 独立创建，并按当前 session 策略持久化。`, proc.Spec.ExitCondition)
	// 3. 执行 Agent，并确保 worklog 记录最终状态。
	response, runErr := r.Agent.RunStream(ctx, messageCtx, input, workLog.OnToken, workLog.OnReasoning)
	workLog.End(runErr)
	// 4. 无论 Agent 成功或失败都写报告；报告写入失败会覆盖本次函数返回错误。
	report := &processReport{ProcessID: proc.ID, WorkLog: workLog.path, ExitCondition: proc.Spec.ExitCondition, Response: response, Err: runErr, StartedAt: started, EndedAt: time.Now().UTC()}
	path, writeErr := r.writeProcessReport("", report)
	proc.ReportPath = path
	if writeErr != nil {
		return writeErr
	}
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	fmt.Fprintf(os.Stdout, "process %s: %s report=%s\n", status, proc.ID, proc.ReportPath)
	return runErr
}

// Close 关闭进程级日志文件。
func (r *Runtime) Close() error {
	logger.CloseLog()
	return nil
}

// =============================================================================
// Runtime 内部 helper：session 和路径
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

// =============================================================================
// 报告和 worklog 辅助：文件命名、渲染、路径清理
// =============================================================================

func (r *Runtime) writeProcessReport(dir string, report *processReport) (string, error) {
	if dir == "" {
		dir = filepath.Join(projectDataDir(r.ProjectDir), "reports")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create process report dir: %w", err)
	}
	ts := time.Now().UTC().Format("20060102-150405.000000000")
	path := filepath.Join(dir, safeName(report.ProcessID)+"."+ts+".md")
	if err := os.WriteFile(path, []byte(renderProcessReport(report)), 0o644); err != nil {
		return "", fmt.Errorf("write process report: %w", err)
	}
	return path, nil
}

func projectDataDir(projectDir string) string {
	return filepath.Join(projectDir, ".walle")
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

func renderProcessReport(report *processReport) string {
	status := "completed"
	errText := ""
	if report.Err != nil {
		status = "failed"
		errText = "\n## Error\n\n```text\n" + report.Err.Error() + "\n```\n"
	}
	return fmt.Sprintf(`# Agent Process Report

- process: %s
- status: %s
- started_at: %s
- ended_at: %s
- worklog: %s

## Exit Condition

%s

## Agent Output

%s
%s`, report.ProcessID, status, report.StartedAt.Format(time.RFC3339), report.EndedAt.Format(time.RFC3339), report.WorkLog, report.ExitCondition, strings.TrimSpace(report.Response), errText)
}

func safeName(raw string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-")
	name := strings.Trim(replacer.Replace(raw), ".-")
	if name == "" {
		return "process"
	}
	return name
}
