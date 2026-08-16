// provider.go - Runtime provider 登录和模型目录的最小入口。
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/lzq/5hAgent/internal/codex"
	"github.com/lzq/5hAgent/internal/utils"
)

// ProviderInfo 是 TUI provider picker 的只读数据。
type ProviderInfo struct {
	Name     string
	LoggedIn bool
}

// Providers 返回内置 OpenAI ChatGPT provider 和用户配置过的 provider。
func (r *Runtime) Providers() []ProviderInfo {
	current, _, _ := strings.Cut(r.ModelRef, "/")
	store, _ := codex.DefaultStore()
	seen := map[string]bool{"openai": true, "deepseek": true}
	providers := []ProviderInfo{
		{Name: "openai", LoggedIn: store != nil && store.LoggedIn()},
		{Name: "deepseek", LoggedIn: true},
	}
	if configured, err := utils.ConfiguredProviders(); err == nil {
		for _, provider := range configured {
			if provider.Name != "" && !seen[provider.Name] {
				seen[provider.Name] = true
				providers = append(providers, ProviderInfo{Name: provider.Name, LoggedIn: true})
			}
		}
	}
	if current != "" && !seen[current] {
		providers = append(providers, ProviderInfo{Name: current, LoggedIn: true})
	}
	sort.SliceStable(providers[1:], func(i, j int) bool { return providers[1+i].Name < providers[1+j].Name })
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
	models, err := r.fetchProviderModels(ctx, provider)
	if err == nil {
		return models, nil
	}
	return nil, err
}

func (r *Runtime) fetchProviderModels(ctx context.Context, provider string) ([]string, error) {
	config, err := utils.LoadConfigWithOptions(utils.LoadConfigOptions{ModelRef: provider + "/__model_catalog__"})
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(strings.TrimRight(config.LLM.BaseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid provider base url %q", config.LLM.BaseURL)
	}
	if config.LLM.Provider == "claude" && !strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/models"
	parsed.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	if config.LLM.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+config.LLM.APIKey)
		request.Header.Set("x-api-key", config.LLM.APIKey)
	}
	if config.LLM.Provider == "claude" {
		request.Header.Set("anthropic-version", "2023-06-01")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("list %s models: %w", provider, err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return nil, fmt.Errorf("list %s models: status %s", provider, response.Status)
	}
	models, err := decodeModelCatalog(response)
	if err != nil {
		return nil, fmt.Errorf("list %s models: %w", provider, err)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("list %s models: empty catalog", provider)
	}
	return models, nil
}

func decodeModelCatalog(response *http.Response) ([]string, error) {
	var payload struct {
		Data   []modelCatalogItem `json:"data"`
		Models []modelCatalogItem `json:"models"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	models := make([]string, 0, len(payload.Data)+len(payload.Models))
	for _, item := range append(payload.Data, payload.Models...) {
		name := ""
		for _, value := range []string{item.ID, item.Name, item.Model, item.Slug} {
			if strings.TrimSpace(value) != "" {
				name = strings.TrimSpace(value)
				break
			}
		}
		if name != "" && !seen[name] {
			seen[name] = true
			models = append(models, name)
		}
	}
	sort.Strings(models)
	return models, nil
}

type modelCatalogItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
	Slug  string `json:"slug"`
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
