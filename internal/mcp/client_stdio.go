// client_stdio.go - MCP Stdio Client 实现
// 通过 stdio 与 MCP 服务器通信，实现 mcp.Client 接口
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lzq/5hAgent/internal/logger"
	"github.com/tidwall/gjson"
)

// JSON-RPC 消息类型
type jsonrpcRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

type jsonrpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

type jsonrpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// StdioClient MCP stdio 传输客户端
type StdioClient struct {
	serverName string
	command    string
	args       []string
	env        map[string]string

	cmd        *exec.Cmd
	stdin      io.Writer
	stdout     *bufio.Reader
	stdoutPipe io.ReadCloser

	mu          sync.RWMutex
	stdinMu     sync.Mutex
	requestID   atomic.Int64
	pendingReqs map[int64]chan *jsonrpcResponse

	initialized atomic.Bool
	tools       []ToolSpec

	ctx    context.Context
	cancel context.CancelFunc
}

// StdioClientConfig 配置
type StdioClientConfig struct {
	Name           string
	Command        string
	Args           []string
	Env            map[string]string
	StartupTimeout time.Duration
}

// NewStdioClient 创建并启动 stdio MCP 客户端
func NewStdioClient(ctx context.Context, config StdioClientConfig) (*StdioClient, error) {
	if config.StartupTimeout == 0 {
		config.StartupTimeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, config.StartupTimeout)
	defer cancel()

	c := &StdioClient{
		serverName:  config.Name,
		command:     config.Command,
		args:        config.Args,
		env:         config.Env,
		pendingReqs: make(map[int64]chan *jsonrpcResponse),
	}

	if err := c.start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP server %s: %w", config.Name, err)
	}

	return c, nil
}

// start 启动 MCP 服务器进程
func (c *StdioClient) start(ctx context.Context) error {
	cmd := exec.Command(c.command, c.args...)

	// 设置环境变量：先继承系统环境，再用自定义值覆盖
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}
	for k, v := range c.env {
		envMap[k] = v
	}
	cmd.Env = make([]string, 0, len(envMap))
	for k, v := range envMap {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// 设置 stdio
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdoutPipe = stdout
	c.stdout = bufio.NewReaderSize(stdout, 64*1024)

	c.ctx, c.cancel = context.WithCancel(context.Background())

	// 启动读取循环
	go c.readLoop()

	// 启动 stderr 读取（避免缓冲区满）
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				// 可以选择打印或丢弃 stderr
				// fmt.Fprintf(os.Stderr, "[MCP %s stderr] %s", c.serverName, string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	// 初始化
	if err := c.initialize(ctx); err != nil {
		c.Close()
		return err
	}

	return nil
}

// readLoop 读取 stdout 并分发响应/通知
func (c *StdioClient) readLoop() {
	defer func() {
		if c.cancel != nil {
			c.cancel()
		}
	}()
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logger.ErrorTag("MCP", "Read error from %s: %v", c.serverName, err)
			}
			return
		}

		if len(line) == 0 {
			continue
		}

		// 解析消息
		lineStr := string(line)
		if gjson.Valid(lineStr) {
			msg := gjson.Parse(lineStr)

			// 检查是否有 id 字段（响应消息 vs 通知）
			if id := msg.Get("id"); id.Exists() {
				idVal := id.Int()

				c.mu.Lock()
				ch, ok := c.pendingReqs[idVal]
				if ok {
					delete(c.pendingReqs, idVal)
				}
				c.mu.Unlock()

				if ok && ch != nil {
					var resp jsonrpcResponse
					if err := json.Unmarshal(line, &resp); err == nil {
						select {
						case ch <- &resp:
						case <-time.After(5 * time.Second):
							// 超时丢弃
						}
					}
				}
			} else {
				// 通知消息（无 id），例如 tools/list_changed
				method := msg.Get("method").String()
				if method == "notifications/tools/list_changed" {
					logger.InfoTag("MCP", "Server %s notified tools changed, refreshing...", c.serverName)
					// 异步刷新，避免阻塞 readLoop 导致死锁
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						if err := c.refreshTools(ctx); err != nil {
							logger.WarnTag("MCP", "Failed to refresh tools from %s: %v", c.serverName, err)
						}
					}()
				} else {
					logger.DebugTag("MCP", "Received notification from %s: %s", c.serverName, method)
				}
			}
		}
	}
}

// initialize 发送初始化请求
func (c *StdioClient) initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"roots":    map[string]bool{"listChanged": true},
			"sampling": map[string]bool{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "5hAgent",
			"version": "0.1.0",
		},
	}

	paramsJSON, _ := json.Marshal(params)

	var resp jsonrpcResponse
	if err := c.sendRequest(ctx, "initialize", paramsJSON, &resp); err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	c.initialized.Store(true)

	// 发送 initialized 通知
	c.sendNotification("initialized", nil)

	// 获取工具列表
	if err := c.refreshTools(ctx); err != nil {
		return fmt.Errorf("failed to get tools: %w", err)
	}

	return nil
}

// refreshTools 刷新工具列表
func (c *StdioClient) refreshTools(ctx context.Context) error {
	var resp jsonrpcResponse
	if err := c.sendRequest(ctx, "tools/list", nil, &resp); err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}

	// 检查结果是否是数组（旧格式）还是对象（新格式）
	resultStr := string(resp.Result)
	if gjson.Valid(resultStr) {
		result := gjson.Parse(resultStr)
		if result.IsArray() {
			// 处理旧格式
			var tools []ToolSpec
			if err := json.Unmarshal(resp.Result, &tools); err == nil {
				c.tools = tools
			}
		} else {
			// 处理新格式 { tools: [...] }
			toolsArr := result.Get("tools").Array()
			tools := make([]ToolSpec, 0, len(toolsArr))
			for _, t := range toolsArr {
				tool := ToolSpec{
					Name:        t.Get("name").Str,
					Description: t.Get("description").Str,
					ReadOnly:    t.Get("readOnly").Bool(),
				}
				if inputSchemaRaw := t.Get("inputSchema").Raw; inputSchemaRaw != "" {
					tool.InputSchema = json.RawMessage(inputSchemaRaw)
				}
				tools = append(tools, tool)
			}
			c.tools = tools
		}
	}

	return nil
}

// CallTool 调用 MCP 工具，实现 Client 接口
func (c *StdioClient) CallTool(ctx context.Context, toolName string, arguments string) (string, error) {
	if !c.initialized.Load() {
		return "", fmt.Errorf("MCP client not initialized")
	}

	// 给工具调用加 30 秒超时，防止 MCP 服务器无响应导致永久阻塞
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	params := map[string]interface{}{
		"name": toolName,
	}

	// arguments 可以是 JSON 字符串或空
	if arguments != "" {
		var argsJSON json.RawMessage
		if err := json.Unmarshal([]byte(arguments), &argsJSON); err != nil {
			// 如果不是有效 JSON，当作参数值传递
			params["arguments"] = arguments
		} else {
			params["arguments"] = argsJSON
		}
	}

	paramsJSON, _ := json.Marshal(params)

	logger.DebugTag("MCP", "Calling %s.%s (timeout 30s)", c.serverName, toolName)

	var resp jsonrpcResponse
	if err := c.sendRequest(callCtx, "tools/call", paramsJSON, &resp); err != nil {
		if callCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("MCP tool call %s timed out after 30s", toolName)
		}
		return "", err
	}

	if resp.Error != nil {
		return "", fmt.Errorf("tool call error: %s", resp.Error.Message)
	}

	// 提取 content 和 isError
	resultStr := string(resp.Result)
	result := gjson.Parse(resultStr)
	isError := result.Get("isError").Bool()
	content := result.Get("content").Raw

	// 解析 content 数组，提取文本
	var texts []string
	var contents []map[string]interface{}
	if err := json.Unmarshal([]byte(content), &contents); err == nil {
		for _, item := range contents {
			// 检查单个 item 的 isError
			if itemIsErr, ok := item["isError"].(bool); ok && itemIsErr {
				isError = true
			}
			if item["type"] == "text" {
				if text, ok := item["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
	}

	if len(texts) > 0 {
		combined := strings.Join(texts, "\n")
		if isError {
			return "", fmt.Errorf("MCP tool error: %s", combined)
		}
		return combined, nil
	}

	if isError {
		return "", fmt.Errorf("MCP tool error: %s", resultStr)
	}
	return resultStr, nil

}

// sendRequest 发送请求并等待响应
func (c *StdioClient) sendRequest(ctx context.Context, method string, params json.RawMessage, resp *jsonrpcResponse) error {
	id := c.requestID.Add(1)

	req := jsonrpcRequest{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request failed: %w", err)
	}

	// 创建响应通道
	ch := make(chan *jsonrpcResponse, 1)
	c.mu.Lock()
	c.pendingReqs[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingReqs, id)
		c.mu.Unlock()
	}()

	// 发送请求（stdinMu 保护并发写入）
	c.stdinMu.Lock()
	_, err = c.stdin.Write(append(reqJSON, '\n'))
	c.stdinMu.Unlock()
	if err != nil {
		return fmt.Errorf("write request failed: %w", err)
	}

	// 等待响应
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return c.ctx.Err()
	case r := <-ch:
		if r == nil {
			return fmt.Errorf("response channel closed")
		}
		*resp = *r
		return nil
	}
}

// sendNotification 发送通知（不等待响应）
func (c *StdioClient) sendNotification(method string, params interface{}) {
	paramsJSON, _ := json.Marshal(params)
	req := jsonrpcRequest{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  paramsJSON,
	}
	reqJSON, _ := json.Marshal(req)
	c.stdinMu.Lock()
	_, err := c.stdin.Write(append(reqJSON, '\n'))
	c.stdinMu.Unlock()
	if err != nil {
		logger.WarnTag("MCP", "Failed to send notification to %s: %v", c.serverName, err)
	}
}

// ListTools 返回可用工具列表
func (c *StdioClient) ListTools() []ToolSpec {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]ToolSpec, len(c.tools))
	copy(result, c.tools)
	return result
}

// Close 关闭连接
func (c *StdioClient) Close() error {
	c.cancel()

	// 关闭 stdout pipe 以中断 readLoop 中的 ReadBytes 阻塞
	if c.stdoutPipe != nil {
		c.stdoutPipe.Close()
	}

	if c.stdin != nil {
		if w, ok := c.stdin.(io.Closer); ok {
			w.Close()
		}
	}

	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	return nil
}

// ServerName 返回服务器名称
func (c *StdioClient) ServerName() string {
	return c.serverName
}

// Ensure 接口实现
var _ Client = (*StdioClient)(nil)
