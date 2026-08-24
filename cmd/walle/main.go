// Command walle 提供 CLI 入口，负责启动 TUI、daemon、进程查询和 attach 命令。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Ozqi/walle/internal/cli"
	agentrt "github.com/Ozqi/walle/internal/runtime"
	"github.com/Ozqi/walle/internal/systemd"
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
	rootCmd.PersistentFlags().StringVarP(&modelRef, "model", "m", "", "Model ref in provider/model format, for example openrouter/openrouter/owl-alpha")
	rootCmd.AddCommand(newPSCommand(), newAttachCommand())
	rootCmd.AddCommand(&cobra.Command{
		Use:   "daemon",
		Short: "Host the attachable interactive Agent",
		Long:  "Host one interactive Agent and expose it through the local supervisor socket.",
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
		return
	}
	if err := tui.LaunchAttachedTUI(cmd.Context(), client); err != nil {
		cli.PrintError(fmt.Errorf("tui error: %w", err))
	}
}

// runDaemon 托管一个可通过本地 Unix Socket attach 的交互 Agent。
func runDaemon(cmd *cobra.Command, args []string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	opts := runtimeOptions()
	opts.PromptBase = "tui"
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
	session := agentrt.NewDaemonSession(ctx, rt)
	control, err := systemd.StartControlServer(ctx, filepath.Join(configDir, "run"), sys, rt.ProjectDir, session)
	if err != nil {
		cli.PrintError(err)
		return
	}
	defer control.Close()
	fmt.Println("interactive agent: interactive")
	<-ctx.Done()
}

func runtimeOptions() agentrt.Options {
	return agentrt.Options{
		Debug:        debugMode,
		SessionID:    sessionID,
		ContinueLast: continueLast,
		LLMFormat:    llmFormat,
		LLMModel:     llmModel,
		ModelRef:     modelRef,
	}
}
