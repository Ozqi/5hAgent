// main.go - 5hAgent 程序入口
// 功能：初始化 Agent、TUI、工具注册，启动交互式对话界面
// 导出函数：main, runInteractive
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	"github.com/lzq/5hAgent/internal/cli"
	"github.com/lzq/5hAgent/internal/commands"
	"github.com/lzq/5hAgent/internal/config"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/llm"
	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/mcp"
	"github.com/lzq/5hAgent/internal/task"
	"github.com/lzq/5hAgent/internal/tools"
	"github.com/lzq/5hAgent/internal/utils"
	"github.com/spf13/cobra"
)

var debugMode bool
var sessionID string
var resumeLast bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "5hagent",
		Short: "5hAgent - A lightweight AI agent framework",
		Long:  `5hAgent is a Go-based AI agent framework powered by Eino, supporting interactive conversations and tool execution.`,
		Run:   runInteractive,
	}
	rootCmd.Flags().BoolVar(&debugMode, "debug", false, "Enable debug mode with verbose logging")
	rootCmd.Flags().StringVar(&sessionID, "session", "", "Resume from existing session ID")
	rootCmd.Flags().BoolVarP(&resumeLast, "resume", "r", false, "Resume from the last session")
	if err := rootCmd.Execute(); err != nil {
		cli.PrintError(err)
		os.Exit(1)
	}
}

func runInteractive(cmd *cobra.Command, args []string) {
	// 加载集中配置
	appConfig, err := config.Load()
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to load configuration: %w", err))
		os.Exit(1)
	}

	// CLI 标志覆盖配置
	if debugMode {
		appConfig.Agent.Debug = true
		logger.SetLevel(logger.DEBUG)
		logger.InfoTag("SYS", "Debug mode enabled")
	}

	ctx := context.Background()

	// *使用启动目录存放 task list 和 sessions（便于项目管理）
	workDir, err := config.GetWorkDir()
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to get working directory: %w", err))
		os.Exit(1)
	}
	taskListPath := filepath.Join(workDir, ".5hagent", "tasks.json")
	sessionsPath := filepath.Join(workDir, ".5hagent", "sessions")

	taskList, err := task.NewTaskList(taskListPath)
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to initialize task list: %w", err))
		os.Exit(1)
	}
	logger.DebugTag("SYS", "Task list initialized at %s", taskListPath)

	// *初始化会话存储
	ctxManager := agentctx.NewManager(sessionsPath)

	// 处理 -r/--resume 参数：自动获取最新会话
	if resumeLast && sessionID == "" {
		sessions, err := ctxManager.ListSessions()
		if err != nil || len(sessions) == 0 {
			logger.InfoTag("SESSION", "No previous session found, creating new one")
			resumeLast = false
		} else {
			sessionID = sessions[0].ID
			logger.InfoTag("SESSION", "Auto-resume last session: %s", sessionID)
		}
	}

	// 处理 --session 参数
	var messageCtx *agentctx.Context
	if sessionID != "" {
		messageCtx, err = ctxManager.CreateContext(sessionID)
		if err != nil {
			cli.PrintError(fmt.Errorf("failed to load session %s: %w", sessionID, err))
			os.Exit(1)
		}
		logger.InfoTag("SESSION", "Resumed session: %s", sessionID)
	} else {
		messageCtx, err = ctxManager.CreateContext("")
		if err != nil {
			cli.PrintError(fmt.Errorf("failed to create context: %w", err))
			os.Exit(1)
		}
		sessionID = ctxManager.GetSessionID(messageCtx)
		logger.InfoTag("SESSION", "Created new session: %s", sessionID)
	}

	// *从配置创建 LLM 客户端
	llmConfig := &llm.Config{
		APIKey:    appConfig.LLM.APIKey,
		BaseURL:   appConfig.LLM.BaseURL,
		Model:     appConfig.LLM.Model,
		MaxTokens: appConfig.LLM.MaxTokens,
	}
	client, err := llm.NewClient(ctx, llmConfig)
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to create LLM client: %w", err))
		os.Exit(1)
	}

	logger.DebugTag("SYS", "Model=%s, BaseURL=%s", llmConfig.Model, llmConfig.BaseURL)

	systemPrompt, err := utils.Load("prompt", "main")
	if err != nil {
		cli.PrintError(fmt.Errorf("failed to get system prompt: %w", err))
		os.Exit(1)
	}
	logger.DebugTag("SYS", "System prompt loaded: %d chars", len(systemPrompt))

	// *从配置创建 Agent
	agentConfig := &agent.Config{
		Name:            appConfig.Agent.Name,
		MaxTotalTokens:  appConfig.Agent.MaxTotalTokens,
		RepeatToolLimit: appConfig.Agent.RepeatToolLimit,
		Debug:           appConfig.Agent.Debug,
		SystemPrompt:    systemPrompt,
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

	// *初始化 MCP 服务器（从 ~/.5hAgent/mcp.json 加载）
	mcpServers, err := commands.LoadMCPServers()
	if err != nil {
		logger.WarnTag("MCP", "Failed to load MCP config: %v", err)
	}
	mcpClients := make([]*mcp.StdioClient, 0, len(mcpServers))
	for _, serverConfig := range mcpServers {
		logger.InfoTag("MCP", "Starting MCP server: %s", serverConfig.Name)

		mcpClient, err := mcp.NewStdioClient(ctx, mcp.StdioClientConfig{
			Name:           serverConfig.Name,
			Command:        serverConfig.Command,
			Args:           serverConfig.Args,
			Env:            serverConfig.Env,
			StartupTimeout: serverConfig.StartupTimeout,
		})
		if err != nil {
			logger.ErrorTag("MCP", "Failed to start MCP server %s: %v", serverConfig.Name, err)
			continue
		}

		mcpClients = append(mcpClients, mcpClient)

		// *注册 MCP 工具
		toolSpecs := mcpClient.ListTools()
		if err := tools.RegisterMCPTools(serverConfig.Name, mcpClient, toolSpecs); err != nil {
			logger.ErrorTag("MCP", "Failed to register tools for %s: %v", serverConfig.Name, err)
			mcpClient.Close()
			continue
		}

		logger.InfoTag("MCP", "Registered %d tools from %s", len(toolSpecs), serverConfig.Name)
	}

	// 设置清理函数
	defer func() {
		for _, client := range mcpClients {
			logger.DebugTag("MCP", "Closing MCP server: %s", client.ServerName())
			if err := client.Close(); err != nil {
				logger.ErrorTag("MCP", "Error closing %s: %v", client.ServerName(), err)
			}
		}
	}()

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

	if err := cli.LaunchTUI(ctx, ag, llmConfig.Model, taskList, ag.GetSkillManager(), ctxManager, messageCtx, sessionID); err != nil {
		cli.PrintError(fmt.Errorf("tui error: %w", err))
		os.Exit(1)
	}
}
