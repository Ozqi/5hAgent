// provider.go - Runtime provider 登录和模型目录的最小入口。
package runtime

import (
	"context"
	"strings"

	"github.com/lzq/5hAgent/internal/codex"
	"github.com/lzq/5hAgent/internal/utils"
)

// ProviderInfo 是 TUI provider picker 的只读数据。
type ProviderInfo struct {
	Name     string
	LoggedIn bool
}

// Providers 返回当前 provider 和内置 OpenAI ChatGPT provider。
func (r *Runtime) Providers() []ProviderInfo {
	current, _, _ := strings.Cut(r.ModelRef, "/")
	store, _ := codex.DefaultStore()
	providers := []ProviderInfo{{Name: "openai", LoggedIn: store != nil && store.LoggedIn()}}
	if current != "" && current != "openai" {
		providers = append(providers, ProviderInfo{Name: current, LoggedIn: true})
	}
	return providers
}

// StartOpenAILogin 启动内置 ChatGPT browser OAuth。
func (r *Runtime) StartOpenAILogin(ctx context.Context) (string, <-chan error, error) {
	store, err := codex.DefaultStore()
	if err != nil {
		return "", nil, err
	}
	return store.StartLogin(ctx)
}

// ProviderModels 返回 provider 当前可选模型。
func (r *Runtime) ProviderModels(ctx context.Context, provider string) ([]string, error) {
	if provider == "openai" {
		store, err := codex.DefaultStore()
		if err != nil {
			return nil, err
		}
		return store.Models(ctx)
	}
	currentProvider, currentModel, _ := strings.Cut(r.ModelRef, "/")
	if currentProvider == provider && currentModel != "" {
		return []string{currentModel}, nil
	}
	return nil, nil
}

func modelLoadOptions(modelRef string) utils.LoadConfigOptions {
	options := utils.LoadConfigOptions{ModelRef: modelRef}
	if strings.HasPrefix(modelRef, "openai/") {
		if store, err := codex.DefaultStore(); err == nil && store.LoggedIn() {
			options.LLMFormat = "codex"
		}
	}
	return options
}
