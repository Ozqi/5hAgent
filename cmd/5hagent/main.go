// main.go - 5hAgent 程序入口
// 功能：提供 TUI 交互入口和无头 runtime 入口，两者共享 internal/runtime 初始化链路。
// 调用方：用户通过 5hagent、5hagent run 启动；测试可直接复用 internal/runtime。
// 全局状态：debugMode/sessionID/continueLast/llmSupplier 等保存 CLI flag 解析结果。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lzq/5hAgent/internal/cli"
	agentrt "github.com/lzq/5hAgent/internal/runtime"
	"github.com/lzq/5hAgent/internal/systemd"
	"github.com/lzq/5hAgent/internal/tui"
	"github.com/lzq/5hAgent/internal/utils"
	"github.com/spf13/cobra"
)

var debugMode bool
var sessionID string
var continueLast bool
var llmFormat string
var llmModel string
var modelRef string
var runTaskID string
var runReportDir string
var runQuiet bool
var daemonPoll time.Duration
var daemonInteractive bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "5hagent",
		Short: "5hAgent - A lightweight AI agent runtime",
		Long:  `5hAgent is a Go-based AI agent runtime powered by Eino. It can run with a TUI or process file-backed tasks headlessly.`,
		Run:   runTUI,
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode with verbose logging")
	rootCmd.PersistentFlags().StringVar(&sessionID, "session", "", "Resume from existing session ID")
	rootCmd.PersistentFlags().BoolVarP(&continueLast, "continue", "c", false, "Resume from the last session")
	rootCmd.PersistentFlags().StringVar(&llmFormat, "llm-format", "", "Temporarily select LLM API format: claude or openai")
	rootCmd.PersistentFlags().StringVar(&llmModel, "llm-model", "", "Temporarily override the selected LLM model")
	rootCmd.PersistentFlags().StringVarP(&modelRef, "model", "m", "", "Model ref in provider/model format, for example openrouter/openrouter/owl-alpha")

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run one file-backed task without launching the TUI",
		Long:  "Run one pending or in-progress task from .5hagent/task.md and write a Markdown report to .5hagent/reports/.",
		Run:   runHeadless,
	}
	runCmd.Flags().StringVar(&runTaskID, "task", "", "Task ID to run; defaults to first in_progress or pending task")
	runCmd.Flags().StringVar(&runReportDir, "report-dir", "", "Directory for Markdown task reports; defaults to .5hagent/reports")
	runCmd.Flags().BoolVar(&runQuiet, "quiet", false, "Suppress headless work log output; only print report path and errors")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(newPSCommand(), newAttachCommand())

	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the Agent Systemd task supervisor",
		Long:  "Run the Agent Systemd loop for .5hagent/task.md. Existing and changed pending/in_progress tasks are started as Agent processes.",
		Run:   runDaemon,
	}
	daemonCmd.Flags().DurationVar(&daemonPoll, "poll", time.Second, "Polling interval for .5hagent/task.md")
	daemonCmd.Flags().BoolVar(&daemonInteractive, "interactive", false, "Host one attachable interactive Agent")
	rootCmd.AddCommand(daemonCmd)

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
		return
	}
	if err := tui.LaunchAttachedTUI(cmd.Context(), client); err != nil {
		cli.PrintError(fmt.Errorf("tui error: %w", err))
	}
}

// runHeadless 执行一个文件任务并写报告。
// 交互边界：输入来自 .5hagent/task.md，输出写入 .5hagent/reports/<task-id>.md。
func runHeadless(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	rt, err := agentrt.New(ctx, runtimeOptions(false))
	if err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
	defer rt.Close()

	report, err := rt.RunTaskOnce(ctx, agentrt.RunOptions{TaskID: runTaskID, ReportDir: runReportDir, WorkLog: !runQuiet})
	if report != nil && report.ReportPath != "" {
		fmt.Printf("report: %s\n", report.ReportPath)
	}
	if err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
}

// runDaemon 启动 Agent Systemd 最小调度循环。
// 交互边界：监听当前项目 .5hagent/task.md，把 in_progress/pending 任务交给 Runtime.RunProcess。
func runDaemon(cmd *cobra.Command, args []string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	opts := runtimeOptions(!daemonInteractive)
	if daemonInteractive {
		opts.PromptBase = "tui"
	}
	rt, err := agentrt.New(ctx, opts)
	if err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
	defer rt.Close()

	configDir, err := utils.GetConfigDir()
	if err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
	sys := systemd.New()
	if daemonInteractive {
		session := agentrt.NewDaemonSession(ctx, rt)
		control, err := systemd.StartControlServer(ctx, filepath.Join(configDir, "run"), sys, rt.ProjectDir, session)
		if err != nil {
			cli.PrintError(err)
			return
		}
		defer control.Close()
		fmt.Printf("interactive agent: daemon-%d/interactive\n", os.Getpid())
		<-ctx.Done()
		return
	}
	control, err := systemd.StartControlServer(ctx, filepath.Join(configDir, "run"), sys, rt.ProjectDir)
	if err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
	defer control.Close()
	sys.StartSource(ctx, agentrt.NewTaskFileEventSource(rt.TaskList, daemonPoll, rt.TaskProcessSpec))
	if err := rt.EmitCurrentTask(sys); err != nil {
		fmt.Fprintf(os.Stderr, "daemon: %v\n", err)
	}
	if err := sys.Run(ctx, rt); err != nil && err != context.Canceled {
		cli.PrintError(err)
		os.Exit(1)
	}
}

func runtimeOptions(memory bool) agentrt.Options {
	return agentrt.Options{
		Debug:         debugMode,
		SessionID:     sessionID,
		ContinueLast:  continueLast,
		MemoryContext: memory,
		LLMFormat:     llmFormat,
		LLMModel:      llmModel,
		ModelRef:      modelRef,
	}
}
