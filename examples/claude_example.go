package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"
	"github.com/lzq/miniAgent/internal/llm"
)

func main() {
	ctx := context.Background()

	// 方式 1: 从环境变量创建客户端（自动加载 .env 文件）
	fmt.Println("=== 方式 1: 从环境变量创建客户端 ===")
	client1, err := llm.NewClientFromEnv(ctx, ".env")
	if err != nil {
		log.Printf("Failed to create client from env: %v\n", err)
	} else {
		fmt.Printf("Client created successfully!\n")
		fmt.Printf("Config: BaseURL=%s, Model=%s\n",
			client1.GetConfig().BaseURL,
			client1.GetConfig().Model)
	}

	// 方式 2: 手动创建配置
	fmt.Println("\n=== 方式 2: 手动创建配置 ===")
	config := &llm.Config{
		APIKey:    "your-api-key-here",
		BaseURL:   "https://api.anthropic.com",
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
	}

	client2, err := llm.NewClient(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// 使用客户端进行对话
	fmt.Println("\n=== 使用客户端进行对话 ===")
	messages := []*schema.Message{
		schema.UserMessage("你好，请用一句话介绍你自己。"),
	}

	model := client2.GetModel()
	response, err := model.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("Failed to generate response: %v", err)
	}

	fmt.Printf("Assistant: %s\n", response.Content)
}
