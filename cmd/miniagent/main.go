// miniAgent 是一个轻量级 AI Agent 框架的命令行工具
// 基于 Go + Eino 框架实现，支持工具调用和交互式对话
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/chzyer/readline"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/cli"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/llm"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/tools"
	"github.com/spf13/cobra"
)

var debugMode bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "miniagent",
		Short: "miniAgent - A lightweight AI agent framework",
		Long:  `miniAgent is a Go-based AI agent framework powered by Eino, supporting interactive conversations and tool execution.`,
		Run:   runInteractive,
	}
	rootCmd.Flags().BoolVar(&debugMode, "debug", false, "Enable debug mode with verbose logging")
	if err := rootCmd.Execute(); err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
}

func runInteractive(cmd *cobra.Command, args []string) {
	// 设置日志级别
	if debugMode {
		logger.SetLevel(logger.DEBUG)
		logger.InfoTag("SYS", "Debug mode enabled")
	}

	ctx := context.Background()

	// 1. 初始化任务列表
	taskListPath := ".5hagent/tasks.json"
	if err := tools.InitTaskList(taskListPath); err != nil {
		cli.PrintError(fmt.Errorf("failed to initialize task list: %w", err))
		os.Exit(1)
	}
	logger.DebugTag("SYS", "Task list initialized at %s", taskListPath)

	// 2. 创建 LLM Client
	logger.DebugTag("SYS", "Creating LLM client from .env")
	client, err := llm.NewClientFromEnv(ctx, ".env")
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create LLM client: %w", err))
		os.Exit(1)
	}

	config := client.GetConfig()
	logger.DebugTag("SYS", "Model=%s, BaseURL=%s", config.Model, config.BaseURL)

	// 3. 获取所有工具
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

	// 4. 绑定工具到模型
	modelWithTools, err := client.GetModel().WithTools(toolInfos)
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to bind tools: %w", err))
		os.Exit(1)
	}

	// 5. 创建 Agent 配置
	agentConfig := &agent.Config{
		Name:         "miniAgent",
		MaxTurns:     10,
		Debug:        debugMode,
		SystemPrompt: "You are a helpful AI assistant.",
	}

	// 6. 创建 Agent
	ag, err := agent.NewAgent(modelWithTools, allTools, agentConfig)
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create agent: %w", err))
		os.Exit(1)
	}

	// 7. 创建 Context Manager 和 Message Context
	ctxManager := agentctx.NewManager()
	messageCtx, err := ctxManager.CreateContext()
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create message context: %w", err))
		os.Exit(1)
	}

	// 8. 创建 readline 实例
	rl, err := readline.New("miniAgent> ")
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create readline: %w", err))
		os.Exit(1)
	}
	defer rl.Close()

	fmt.Println(logger.Bold(logger.Magenta("miniAgent - Interactive Mode (Stage 1)")))
	fmt.Println(logger.Gray("Type your message and press Enter. Ctrl+C or Ctrl+D to exit."))
	fmt.Println()

	// 9. REPL 循环
	for {
		line, err := rl.Readline()
		if err != nil {
			if err == io.EOF || err == readline.ErrInterrupt {
				fmt.Println(logger.Yellow("\nGoodbye!"))
				break
			}
			cli.PrintError(fmt.Errorf("readline error: %w", err))
			continue
		}

		if line == "" {
			continue
		}

		logger.DebugTag("USER", "Input: %s", line)

		fmt.Print(logger.Gray("\n[...]"))

		// 运行 Agent 流式输出
		firstToken := true
		_, err = ag.RunStream(ctx, messageCtx, line, func(token string) {
			if firstToken {
				// 清除 "思考中..." 提示，回到行首
				fmt.Print("\r")
				for i := 0; i < 20; i++ {
					fmt.Print(" ")
				}
				fmt.Print("\r" + logger.Bold(logger.Cyan("Assistant: ")))
				firstToken = false
			}
			fmt.Print(token)
		})

		if err != nil {
			// 清除思考提示
			fmt.Print("\r")
			for i := 0; i < 20; i++ {
				fmt.Print(" ")
			}
			fmt.Print("\r")
			logger.ErrorTag("AGENT", "Error: %v", err)
			cli.PrintError(fmt.Errorf("agent error: %w", err))
			continue
		}

		logger.DebugTag("AGENT", "Response completed")

		fmt.Println()
	}
}
