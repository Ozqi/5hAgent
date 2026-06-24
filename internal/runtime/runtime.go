// runtime.go - 无头 Agent 运行时
// 功能：集中初始化配置、LLM、Agent、工具、MCP，并提供基于任务文件的无头执行入口。
// 调用方：cmd/5hagent/main.go 的 TUI 入口和 headless run 子命令。
// 全局状态：复用 logger 和 tools 包内注册表；Runtime.Close 负责释放 MCP 进程和日志句柄。
package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/commands"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/llm"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/mcp"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/tools"
	"github.com/lzq/5hAgent/internal/utils"
)

// Options 控制运行时初始化方式。
// Debug 会提升日志级别；SessionID/ContinueLast 只影响默认 MessageCtx，headless task 可继续创建独立上下文。
type Options struct {
	Debug        bool
	SessionID    string
	ContinueLast bool
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
	ModelName  string

	mcpClients []*mcp.StdioClient // 需要随进程退出释放的 MCP stdio 子进程
}

// RunOptions 描述一次文件任务执行。
// TaskID 为空时按 in_progress -> pending 顺序选择一个任务；ReportDir 为空时写入 .5hagent/reports。
type RunOptions struct {
	TaskID    string
	ReportDir string
	WorkLog   bool // 是否在 headless 模式输出可读工作日志
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

// New 初始化一个可交互或无头复用的 Runtime。
// 步骤：加载配置 -> 初始化日志 -> 打开任务文件和 session store -> 创建 LLM/Agent -> 注册本地与 MCP 工具。
// 副作用：创建 ~/.5hAgent、项目 .5hagent、日志文件，可能启动 MCP stdio 子进程。
func New(ctx context.Context, opts Options) (*Runtime, error) {
	appConfig, err := utils.LoadConfig()
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

	projectDataDir, err := utils.GetProjectDataDir()
	if err != nil {
		return nil, fmt.Errorf("get project data directory: %w", err)
	}
	list, err := task.NewTaskList(filepath.Join(projectDataDir, "task.md"))
	if err != nil {
		return nil, fmt.Errorf("initialize task list: %w", err)
	}

	configDir, err := utils.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config directory: %w", err)
	}
	ctxManager := agentctx.NewManager(filepath.Join(configDir, "sessions"))
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
	systemPrompt, err := utils.LoadSystemPrompt(promptDir, appConfig.LLM.Provider, appConfig.LLM.Model)
	if err != nil {
		return nil, fmt.Errorf("load system prompt: %w", err)
	}

	ag, err := agent.NewAgent(nil, nil, &agent.Config{
		Name:                appConfig.Agent.Name,
		MaxTotalTokens:      appConfig.Agent.MaxTotalTokens,
		RepeatToolLimit:     appConfig.Agent.RepeatToolLimit,
		ContextAutoCompress: appConfig.Agent.ContextAutoCompress,
		Debug:               appConfig.Agent.Debug,
		SystemPrompt:        systemPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	ag.SetCtxManager(ctxManager)

	if err := tools.InitRegistry(list, ag.GetSkillManager()); err != nil {
		return nil, fmt.Errorf("init tools: %w", err)
	}
	tools.RegisterContextTool(client.GetModel(), promptDir)
	mcpClients := startMCPServers(ctx)

	allTools := tools.GetAllTools()
	toolInfos := make([]*schema.ToolInfo, 0, len(allTools))
	for _, t := range allTools {
		info, err := t.Info(ctx)
		if err != nil {
			logger.WarnTag("TOOL", "skip tool info: %v", err)
			continue
		}
		toolInfos = append(toolInfos, info)
	}
	modelWithTools, err := client.GetModel().WithTools(toolInfos)
	if err != nil {
		return nil, fmt.Errorf("bind tools: %w", err)
	}
	ag.SetModel(modelWithTools)
	ag.SetTools(allTools)

	return &Runtime{
		Agent:      ag,
		TaskList:   list,
		CtxManager: ctxManager,
		MessageCtx: messageCtx,
		SessionID:  sessionID,
		PromptDir:  promptDir,
		ModelName:  llmConfig.Model,
		mcpClients: mcpClients,
	}, nil
}

// Close 释放 Runtime 启动的外部资源。
func (r *Runtime) Close() error {
	var firstErr error
	for _, client := range r.mcpClients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	logger.CloseDebugLog()
	return firstErr
}

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
	projectDataDir, err := utils.GetProjectDataDir()
	if err != nil {
		return nil, fmt.Errorf("get project data directory: %w", err)
	}
	workLog := newHeadlessWorkLog(opts.WorkLog, projectDataDir, r.Agent.Name(), started)
	workLog.Start(selected)
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

func startMCPServers(ctx context.Context) []*mcp.StdioClient {
	servers, err := commands.LoadMCPServers()
	if err != nil {
		logger.WarnTag("MCP", "load MCP config: %v", err)
		return nil
	}
	clients := make([]*mcp.StdioClient, 0, len(servers))
	for _, cfg := range servers {
		client, err := mcp.NewStdioClient(ctx, mcp.StdioClientConfig{
			Name:           cfg.Name,
			Command:        cfg.Command,
			Args:           cfg.Args,
			Env:            cfg.Env,
			StartupTimeout: cfg.StartupTimeout,
		})
		if err != nil {
			logger.ErrorTag("MCP", "start %s: %v", cfg.Name, err)
			continue
		}
		if err := tools.RegisterMCPTools(cfg.Name, client, client.ListTools()); err != nil {
			logger.ErrorTag("MCP", "register %s: %v", cfg.Name, err)
			_ = client.Close()
			continue
		}
		clients = append(clients, client)
	}
	return clients
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
1. 先读取必要文件和任务上下文，再行动。
2. 如需修改代码，保持极简 baseline，优先复用已有模块。
3. 完成后给出可写入执行报告的简短结果、证据和后续建议。`, taskPath, t.ID, t.Title, t.Status, t.Description)
}

func (r *Runtime) writeReport(dir string, report *RunReport) (string, error) {
	if dir == "" {
		dataDir, err := utils.GetProjectDataDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(dataDir, "reports")
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

func safeName(raw string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-")
	name := strings.Trim(replacer.Replace(raw), ".-")
	if name == "" {
		return "task"
	}
	return name
}
