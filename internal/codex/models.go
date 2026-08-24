package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
)

// Models 返回账号目录中去重并排序后的模型标识，401 时强制刷新 token 后重试一次。
func (s *Store) Models(ctx context.Context) ([]string, error) {
	return s.models(ctx, true)
}

func (s *Store) models(ctx context.Context, retry bool) ([]string, error) {
	// 1. 获取有效 token，并携带 account ID 请求模型目录。
	accessToken, accountID, err := s.Token(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := codexBaseURL + "/models?" + url.Values{"client_version": {clientVersion}}.Encode()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if accountID != "" {
		request.Header.Set("ChatGPT-Account-ID", accountID)
	}
	request.Header.Set("Originator", "walle")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("list Codex models: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		if response.StatusCode == http.StatusUnauthorized && retry {
			_ = s.ForceRefresh(ctx)
			return s.models(ctx, false)
		}
		return nil, fmt.Errorf("list Codex models: status %s", response.Status)
	}
	// 2. 兼容目录项的多个名称字段，按优先级选取首个非空值并去重排序。
	var payload struct {
		Models []struct {
			Slug  string `json:"slug"`
			ID    string `json:"id"`
			Model string `json:"model"`
			Name  string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(payload.Models))
	models := make([]string, 0, len(payload.Models))
	for _, item := range payload.Models {
		name := item.Slug
		if name == "" {
			name = item.ID
		}
		if name == "" {
			name = item.Model
		}
		if name == "" {
			name = item.Name
		}
		if name != "" {
			if _, exists := seen[name]; !exists {
				seen[name] = struct{}{}
				models = append(models, name)
			}
		}
	}
	sort.Strings(models)
	return models, nil
}
