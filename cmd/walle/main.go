// Command walle 提供 CLI 入口，负责启动 TUI、daemon、进程查询和 attach 命令。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/Ozqi/walle/internal/agentd"
	"github.com/Ozqi/walle/internal/cli"
	agentrt "github.com/Ozqi/walle/internal/runtime"
	"github.com/Ozqi/walle/internal/tui"
	"github.com/Ozqi/walle/internal/utils"
	"github.com/spf13/cobra"
)

var debugMode bool
var sessionID string
var continueLast bool
var llmFormat string
var llmModel string
var modelRef string

func main() {
	// 1. 注册根命令、全局模型参数和各运行模式子命令。
	rootCmd := &cobra.Command{
		Use:   "walle",
		Short: "walle - A lightweight AI agent runtime",
		Long:  `walle 是一个用 Go 和 Eino 实现的轻量 Agent Runtime，会使用工具、修补自己，并运行命令验证结果。`,
		Run:   runTUI,
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode with verbose logging")
	rootCmd.PersistentFlags().StringVar(&sessionID, "session", "", "Resume from existing session ID")
	rootCmd.PersistentFlags().BoolVarP(&continueLast, "continue", "c", false, "Resume from the last session")
	rootCmd.PersistentFlags().StringVar(&llmFormat, "llm-format", "", "Temporarily select LLM API format: claude or openai")
	rootCmd.PersistentFlags().StringVar(&llmModel, "llm-model", "", "Temporarily override the selected LLM model")
	rootCmd.PersistentFlags().StringVarP(&modelRef, "model", "m", "", "Temporarily select model as provider/model")
	rootCmd.AddCommand(newPSCommand(), newAttachCommand())
	rootCmd.AddCommand(&cobra.Command{
		Use:   "daemon",
		Short: "Host attachable interactive Agents",
		Long:  "Host workspace interactive Agents and expose them through the local supervisor socket.",
		Run:   runDaemon,
	})

	// 2. 执行 Cobra 分发；命令失败统一输出并设置进程退出码。
	if err := rootCmd.Execute(); err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
}

// runTUI 启动独立 daemon Agent，并把当前终端作为可分离 TUI 客户端接入。
func runTUI(cmd *cobra.Command, args []string) {
	client, err := startInteractiveClient(cmd.Context())
	if err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
	if err := tui.LaunchAttachedTUI(cmd.Context(), client); err != nil {
		cli.PrintError(fmt.Errorf("tui error: %w", err))
		os.Exit(1)
	}
}

// runDaemon 启动用户级 supervisor，按 open 请求创建 workspace interactive Runtime。
func runDaemon(cmd *cobra.Command, args []string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	configDir, err := utils.GetConfigDir()
	if err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
	sys := agentd.New()
	registry := newInteractiveRegistry(ctx)
	defer registry.Close()
	control, err := agentd.StartControlServer(ctx, filepath.Join(configDir, "run"), sys, "", registry)
	if err != nil {
		cli.PrintError(err)
		return
	}
	defer control.Close()
	fmt.Println("interactive supervisor ready")
	<-ctx.Done()
}

type interactiveRegistry struct {
	ctx      context.Context
	mu       sync.Mutex
	nextID   int
	sessions map[string]*agentrt.DaemonSession
}

func newInteractiveRegistry(ctx context.Context) *interactiveRegistry {
	return &interactiveRegistry{ctx: ctx, sessions: make(map[string]*agentrt.DaemonSession)}
}

func (r *interactiveRegistry) OpenInteractive(ctx context.Context, req agentd.OpenRequest) (agentd.InteractiveProcess, error) {
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	r.mu.Lock()
	if req.SessionID != "" {
		if session := r.findSessionLocked(workspace, req.SessionID, false); session != nil {
			r.mu.Unlock()
			return session, nil
		}
	} else if req.Continue {
		if session := r.findSessionLocked(workspace, "", true); session != nil {
			r.mu.Unlock()
			return session, nil
		}
	}
	r.nextID++
	id := fmt.Sprintf("interactive-%d", r.nextID)
	r.mu.Unlock()

	opts := agentrt.Options{
		Debug:        req.Debug,
		SessionID:    req.SessionID,
		ContinueLast: req.Continue && req.SessionID == "",
		ProjectDir:   workspace,
		LLMFormat:    req.LLMFormat,
		LLMModel:     req.LLMModel,
		ModelRef:     req.ModelRef,
		PromptBase:   req.PromptBase,
	}
	if opts.PromptBase == "" {
		opts.PromptBase = "tui"
	}
	rt, err := agentrt.New(ctx, opts)
	if err != nil {
		return nil, err
	}
	session := agentrt.NewDaemonSession(r.ctx, rt, id, workspaceName(workspace))
	r.mu.Lock()
	r.sessions[id] = session
	r.mu.Unlock()
	return session, nil
}

func (r *interactiveRegistry) ListInteractive() []agentd.ProcessSnapshot {
	r.mu.Lock()
	processes := make([]agentd.ProcessSnapshot, 0, len(r.sessions))
	for _, session := range r.sessions {
		processes = append(processes, session.Snapshot())
	}
	r.mu.Unlock()
	sort.Slice(processes, func(i, j int) bool { return processes[i].StartedAt.Before(processes[j].StartedAt) })
	return processes
}

func (r *interactiveRegistry) FindInteractive(id string) agentd.InteractiveProcess {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[id]
}

func (r *interactiveRegistry) Close() {
	r.mu.Lock()
	sessions := make([]*agentrt.DaemonSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
}

func (r *interactiveRegistry) findSessionLocked(workspace string, sessionID string, idleOnly bool) *agentrt.DaemonSession {
	var latest *agentrt.DaemonSession
	var latestSnapshot agentd.ProcessSnapshot
	for _, session := range r.sessions {
		snapshot := session.Snapshot()
		if snapshot.Workspace != workspace {
			continue
		}
		if sessionID != "" && snapshot.SessionID != sessionID {
			continue
		}
		if idleOnly && snapshot.State != agentd.ProcessIdle {
			continue
		}
		if latest == nil || snapshot.StartedAt.After(latestSnapshot.StartedAt) {
			latest = session
			latestSnapshot = snapshot
		}
	}
	return latest
}

func workspaceName(workspace string) string {
	name := filepath.Base(workspace)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return workspace
	}
	return name
}
