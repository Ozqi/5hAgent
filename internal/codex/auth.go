// auth.go - Codex ChatGPT OAuth、凭据存储和刷新。
// 凭据只保存在用户目录，调用方只能获取短期 access token 和 account id。
package codex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lzq/5hAgent/internal/utils"
)

const (
	clientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	authIssuer    = "https://auth.openai.com"
	authScope     = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	codexBaseURL  = "https://chatgpt.com/backend-api/codex"
	clientVersion = "0.0.0"
	refreshWindow = 5 * time.Minute
	loginTimeout  = 10 * time.Minute
)

var (
	defaultStoreOnce sync.Once
	defaultStore     *Store
	defaultStoreErr  error
)

// Credentials 是 5hAgent 自己持久化的 ChatGPT OAuth 凭据。
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	AccountID    string    `json:"account_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Store 串行化登录、读取和 refresh token 写回。
type Store struct {
	mu     sync.Mutex
	path   string
	client *http.Client
}

// DefaultStore 返回用户级 Codex 凭据仓库。
func DefaultStore() (*Store, error) {
	defaultStoreOnce.Do(func() {
		dir, err := utils.GetConfigDir()
		if err != nil {
			defaultStoreErr = err
			return
		}
		defaultStore = &Store{path: filepath.Join(dir, "auth", "codex.json"), client: &http.Client{}}
	})
	return defaultStore, defaultStoreErr
}

// LoggedIn 报告是否存在可读取的 ChatGPT 凭据。
func (s *Store) LoggedIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.load()
	return err == nil && credentials.AccessToken != "" && credentials.RefreshToken != ""
}

// StartLogin 启动短期 localhost OAuth callback，立即返回可点击 URL 和完成通道。
func (s *Store) StartLogin(ctx context.Context) (string, <-chan error, error) {
	listener, port, err := listenCallback()
	if err != nil {
		return "", nil, err
	}
	verifier, err := randomURLToken(32)
	if err != nil {
		listener.Close()
		return "", nil, err
	}
	state, err := randomURLToken(32)
	if err != nil {
		listener.Close()
		return "", nil, err
	}
	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", port)
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"response_type":              {"code"},
		"client_id":                  {clientID},
		"redirect_uri":               {redirectURI},
		"scope":                      {authScope},
		"code_challenge":             {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method":      {"S256"},
		"state":                      {state},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"originator":                 {"5hagent"},
	}
	authURL := authIssuer + "/oauth/authorize?" + query.Encode()
	done := make(chan error, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	finished := make(chan struct{})
	var finishOnce sync.Once
	finish := func() {
		finishOnce.Do(func() {
			close(finished)
			go server.Shutdown(context.Background())
		})
	}
	var completeOnce sync.Once
	complete := func(err error) {
		completeOnce.Do(func() {
			done <- err
			finish()
		})
	}
	server.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/auth/callback" {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Query().Get("state") != state {
			err := fmt.Errorf("OAuth state mismatch")
			http.Error(writer, err.Error(), http.StatusBadRequest)
			complete(err)
			return
		}
		code := request.URL.Query().Get("code")
		if code == "" {
			err := fmt.Errorf("OAuth callback missing code: %s", request.URL.Query().Get("error"))
			http.Error(writer, err.Error(), http.StatusBadRequest)
			complete(err)
			return
		}
		if err := s.exchange(request.Context(), code, verifier, redirectURI); err != nil {
			http.Error(writer, "Login failed; return to 5hAgent", http.StatusBadGateway)
			complete(err)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(writer, "<h2>5hAgent login complete</h2><p>You can close this page.</p>")
		complete(nil)
	})
	go func() {
		go func() {
			timer := time.NewTimer(loginTimeout)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				complete(ctx.Err())
			case <-timer.C:
				complete(fmt.Errorf("OAuth login timed out"))
			case <-finished:
			}
		}()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			complete(err)
		}
	}()
	openBrowser(authURL)
	return authURL, done, nil
}

// ForceRefresh 刷新可能被服务端提前撤销的 access token。
func (s *Store) ForceRefresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.load()
	if err != nil {
		return err
	}
	credentials, err = s.refresh(ctx, credentials)
	if err != nil {
		return err
	}
	return s.save(credentials)
}

// Token 返回有效 access token；临近过期时自动刷新并原子写回。
func (s *Store) Token(ctx context.Context) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.load()
	if err != nil {
		return "", "", err
	}
	if time.Until(credentials.ExpiresAt) <= refreshWindow {
		credentials, err = s.refresh(ctx, credentials)
		if err != nil {
			return "", "", err
		}
		if err := s.save(credentials); err != nil {
			return "", "", err
		}
	}
	return credentials.AccessToken, credentials.AccountID, nil
}

func (s *Store) exchange(ctx context.Context, code string, verifier string, redirectURI string) error {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, authIssuer+"/oauth/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("exchange OAuth code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("exchange OAuth code: status %s", response.Status)
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return fmt.Errorf("decode OAuth token: %w", err)
	}
	if token.AccessToken == "" || token.RefreshToken == "" {
		return fmt.Errorf("OAuth token response missing access or refresh token")
	}
	if token.ExpiresIn == 0 {
		token.ExpiresIn = 3600
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(Credentials{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, IDToken: token.IDToken, AccountID: accountID(token.IDToken, token.AccessToken), ExpiresAt: time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)})
}

func (s *Store) refresh(ctx context.Context, credentials Credentials) (Credentials, error) {
	body, _ := json.Marshal(map[string]string{"client_id": clientID, "grant_type": "refresh_token", "refresh_token": credentials.RefreshToken})
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, authIssuer+"/oauth/token", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return credentials, fmt.Errorf("refresh Codex token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return credentials, fmt.Errorf("refresh Codex token: status %s", response.Status)
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return credentials, err
	}
	if token.AccessToken != "" {
		credentials.AccessToken = token.AccessToken
	}
	if token.RefreshToken != "" {
		credentials.RefreshToken = token.RefreshToken
	}
	if token.IDToken != "" {
		credentials.IDToken = token.IDToken
	}
	if id := accountID(token.IDToken, token.AccessToken); id != "" {
		credentials.AccountID = id
	}
	if token.ExpiresIn == 0 {
		token.ExpiresIn = 3600
	}
	credentials.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return credentials, nil
}

func (s *Store) load() (Credentials, error) {
	var credentials Credentials
	data, err := os.ReadFile(s.path)
	if err != nil {
		return credentials, fmt.Errorf("Codex is not logged in")
	}
	if err := json.Unmarshal(data, &credentials); err != nil {
		return credentials, fmt.Errorf("decode Codex credentials: %w", err)
	}
	return credentials, nil
}

func (s *Store) save(credentials Credentials) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".codex-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}

func listenCallback() (net.Listener, int, error) {
	for _, port := range []int{1455, 1457} {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, fmt.Errorf("OAuth callback ports 1455 and 1457 are unavailable")
}

func randomURLToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func accountID(tokens ...string) string {
	for _, token := range tokens {
		parts := strings.Split(token, ".")
		if len(parts) < 2 {
			continue
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		var claims map[string]any
		if json.Unmarshal(payload, &claims) == nil {
			if id := accountIDClaim(claims); id != "" {
				return id
			}
		}
	}
	return ""
}

func accountIDClaim(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if text, ok := item.(string); ok && text != "" && (key == "account_id" || strings.Contains(key, "chatgpt_account_id")) {
				return text
			}
			if id := accountIDClaim(item); id != "" {
				return id
			}
		}
	case []any:
		for _, item := range typed {
			if id := accountIDClaim(item); id != "" {
				return id
			}
		}
	}
	return ""
}

func openBrowser(target string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	_ = command.Start()
}
