// main.go - 5hAgent 程序入口
// 功能：初始化 Agent、TUI、工具注册，启动交互式对话界面
// 导出函数：main, runInteractive
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/cli"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/llm"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/tools"
	"github.com/lzq/5hAgent/internal/utils"
	"github.com/spf13/cobra"
)

var debugMode bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "5hagent",
		Short: "5hAgent - A lightweight AI agent framework",
		Long:  `5hAgent is a Go-based AI agent framework powered by Eino, supporting interactive conversations and tool execution.`,
		Run:   runInteractive,
	}
	rootCmd.Flags().BoolVar(&debugMode, "debug", false, "Enable debug mode with verbose logging")
	if err := rootCmd.Execute(); err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
}

func runInteractive(cmd *cobra.Command, args []string) {
	if debugMode {
		logger.SetLevel(logger.DEBUG)
		logger.InfoTag("SYS", "Debug mode enabled")
	}

	ctx := context.Background()

	taskListPath := "task.md"
	taskList, err := task.NewTaskList(taskListPath)
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to initialize task list: %w", err))
		os.Exit(1)
	}
	logger.DebugTag("SYS", "Task list initialized at %s", taskListPath)

	client, err := llm.NewClientFromEnv(ctx, ".env")
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create LLM client: %w", err))
		os.Exit(1)
	}

	config := client.GetConfig()
	logger.DebugTag("SYS", "Model=%s, BaseURL=%s", config.Model, config.BaseURL)

	systemPrompt, err := utils.Load("prompt", "main")
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to get system prompt: %w", err))
		os.Exit(1)
	}
	logger.DebugTag("SYS", "System prompt loaded: %d chars", len(systemPrompt))

	agentConfig := &agent.Config{
		Name:           "5hAgent",
		MaxTotalTokens: 200000,
		Debug:          debugMode,
		SystemPrompt:   systemPrompt,
	}

	ag, err := agent.NewAgent(nil, nil, agentConfig)
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create agent: %w", err))
		os.Exit(1)
	}

	if err := tools.InitRegistry(taskList, ag.GetSkillManager()); err != nil {
		cli.PrintError(fmt.Errorf("failed to init tools: %w", err))
		os.Exit(1)
	}

	allTools := tools.GetAllTools()
	toolInfos := make([]*schema.ToolInfo, 0, len(allTools))
	for _, t := range allTools {
		info, err := t.Info(ctx)
		if err != nil {
			cli.PrintError(fmt.Errorf("failed to get tool info: %w", err))
			continue
		}
		toolInfos = append(toolInfos, info)
	}

	modelWithTools, err := client.GetModel().WithTools(toolInfos)
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to bind tools: %w", err))
		os.Exit(1)
	}

	ag.SetModel(modelWithTools)
	ag.SetTools(allTools)

	ctxManager := agentctx.NewManager()
	messageCtx, err := ctxManager.CreateContext()
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create message context: %w", err))
		os.Exit(1)
	}

	if err := cli.LaunchTUI(ctx, ag, config.Model, taskList, ag.GetSkillManager(), ctxManager, messageCtx); err != nil {
		cli.PrintError(fmt.Errorf("tui error: %w", err))
		os.Exit(1)
	}
}
