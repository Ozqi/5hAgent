// control.go - Agent Systemd 本地 Unix Socket 控制通道。
// daemon 通过 NDJSON 暴露进程列表和可重连的交互 Agent；CLI 用同一协议实现 ps/attach。
package systemd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const supervisorSocket = "supervisor.sock"

// ProcessSnapshot 是跨进程查询使用的只读 AgentProcess 快照。
type ProcessSnapshot struct {
	ID          string       `json:"id"`
	State       ProcessState `json:"state"`
	TaskID      string       `json:"task_id,omitempty"`
	TaskTitle   string       `json:"task_title,omitempty"`
	StartedAt   time.Time    `json:"started_at"`
	Workspace   string       `json:"workspace"`
	WorkLogPath string       `json:"worklog_path,omitempty"`
	Model       string       `json:"model,omitempty"`
	SessionID   string       `json:"session_id,omitempty"`
	Interactive bool         `json:"interactive,omitempty"`
}

// ProcessEvent 是 daemon 向 attached TUI 推送的结构化事件。
type ProcessEvent struct {
	Seq     uint64   `json:"seq"`
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Name    string   `json:"name,omitempty"`
	Args    string   `json:"args,omitempty"`
	Result  string   `json:"result,omitempty"`
	Error   string   `json:"error,omitempty"`
	Busy    bool     `json:"busy,omitempty"`
	Options []string `json:"options,omitempty"`
}

// InteractiveProcess 是控制通道依赖的最小长驻 Agent 接口。
type InteractiveProcess interface {
	Snapshot() ProcessSnapshot
	Attach() ([]ProcessEvent, <-chan ProcessEvent, func())
	Submit(string) error
	Stop() error
}

type controlMessage struct {
	Type      string            `json:"type"`
	ID        uint64            `json:"id,omitempty"`
	ProcessID string            `json:"process_id,omitempty"`
	Text      string            `json:"text,omitempty"`
	Processes []ProcessSnapshot `json:"processes,omitempty"`
	Process   *ProcessSnapshot  `json:"process,omitempty"`
	Event     *ProcessEvent     `json:"event,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// Processes 返回当前运行中进程的副本，不外泄进程表中的可变指针。
func (s *AgentSystemd) Processes(workspace string) []ProcessSnapshot {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var processes []ProcessSnapshot
	for _, proc := range s.processes {
		if proc == nil || proc.State != ProcessRunning {
			continue
		}
		processes = append(processes, ProcessSnapshot{
			ID: proc.ID, State: proc.State, TaskID: proc.SourceTask.ID, TaskTitle: proc.SourceTask.Title,
			StartedAt: proc.StartedAt, Workspace: workspace, WorkLogPath: proc.workLogPath(),
		})
	}
	sort.Slice(processes, func(i, j int) bool { return processes[i].StartedAt.Before(processes[j].StartedAt) })
	return processes
}

// ControlServer 向同一用户的 CLI 暴露 daemon 进程。
type ControlServer struct {
	listener net.Listener
	path     string
	once     sync.Once
}

// StartControlServer 在 controlDir 创建用户级唯一 supervisor socket。
func StartControlServer(ctx context.Context, controlDir string, sys *AgentSystemd, workspace string, interactive ...InteractiveProcess) (*ControlServer, error) {
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return nil, fmt.Errorf("create daemon control dir: %w", err)
	}
	oldSockets, _ := filepath.Glob(filepath.Join(controlDir, "daemon-*.sock"))
	for _, old := range oldSockets {
		_ = os.Remove(old)
	}
	path := filepath.Join(controlDir, supervisorSocket)
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen daemon control socket: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	server := &ControlServer{listener: listener, path: path}
	go server.serve(ctx, sys, workspace, interactive)
	return server, nil
}

func (s *ControlServer) serve(ctx context.Context, sys *AgentSystemd, workspace string, interactive []InteractiveProcess) {
	go func() { <-ctx.Done(); _ = s.Close() }()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go handleControlConn(conn, sys, workspace, interactive)
	}
}

func handleControlConn(conn net.Conn, sys *AgentSystemd, workspace string, interactive []InteractiveProcess) {
	defer conn.Close()
	dec, enc := json.NewDecoder(conn), json.NewEncoder(conn)
	var first controlMessage
	if err := dec.Decode(&first); err != nil {
		return
	}
	if first.Type == "list" {
		processes := sys.Processes(workspace)
		for _, proc := range interactive {
			processes = append(processes, proc.Snapshot())
		}
		_ = enc.Encode(controlMessage{Type: "list", Processes: processes})
		return
	}
	if first.Type != "attach" {
		_ = enc.Encode(controlMessage{Type: "error", Error: "expected list or attach"})
		return
	}
	var target InteractiveProcess
	for _, proc := range interactive {
		if proc.Snapshot().ID == first.ProcessID {
			target = proc
			break
		}
	}
	if target == nil {
		_ = enc.Encode(controlMessage{Type: "error", Error: "interactive process not found"})
		return
	}
	history, events, detach := target.Attach()
	defer detach()
	snapshot := target.Snapshot()
	if err := enc.Encode(controlMessage{Type: "attached", Process: &snapshot}); err != nil {
		return
	}
	for i := range history {
		if err := enc.Encode(controlMessage{Type: "event", Event: &history[i]}); err != nil {
			return
		}
	}
	if err := enc.Encode(controlMessage{Type: "ready"}); err != nil {
		return
	}
	requests := make(chan controlMessage)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(requests)
		for {
			var request controlMessage
			if dec.Decode(&request) != nil {
				return
			}
			select {
			case requests <- request:
			case <-done:
				return
			}
		}
	}()
	for {
		select {
		case event, ok := <-events:
			if !ok || enc.Encode(controlMessage{Type: "event", Event: &event}) != nil {
				return
			}
		case request, ok := <-requests:
			if !ok || request.Type == "detach" {
				return
			}
			if request.Type == "input" {
				response := controlMessage{Type: "input_result", ID: request.ID}
				if err := target.Submit(request.Text); err != nil {
					response.Error = err.Error()
				}
				if enc.Encode(response) != nil {
					return
				}
				continue
			}
			if request.Type == "stop" {
				response := controlMessage{Type: "stop_result", ID: request.ID}
				if err := target.Stop(); err != nil {
					response.Error = err.Error()
				}
				if enc.Encode(response) != nil {
					return
				}
			}
		}
	}
}

// Close 关闭控制 socket 并删除 socket 文件。
func (s *ControlServer) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.once.Do(func() {
		err = s.listener.Close()
		_ = os.Remove(s.path)
	})
	return err
}

// ListProcesses 查询用户级唯一 supervisor 中的进程。
func ListProcesses(controlDir string) ([]ProcessSnapshot, error) {
	path := filepath.Join(controlDir, supervisorSocket)
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		_ = os.Remove(path)
		return nil, nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if err := json.NewEncoder(conn).Encode(controlMessage{Type: "list"}); err != nil {
		return nil, err
	}
	var response controlMessage
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return nil, err
	}
	if response.Type != "list" {
		return nil, fmt.Errorf("unexpected supervisor response %q", response.Type)
	}
	processes := response.Processes
	sort.Slice(processes, func(i, j int) bool { return processes[i].StartedAt.Before(processes[j].StartedAt) })
	return processes, nil
}

// ProcessClient 是 attached TUI 使用的双向 NDJSON 客户端。
type ProcessClient struct {
	conn     net.Conn
	enc      *json.Encoder
	mu       sync.Mutex
	once     sync.Once
	done     chan struct{}
	nextID   uint64
	pending  map[uint64]chan error
	snapshot ProcessSnapshot
	events   chan ProcessEvent
}

// AttachProcess 连接 target 对应的 daemon 并订阅交互 Agent 事件。
func AttachProcess(controlDir string, target string) (*ProcessClient, error) {
	conn, err := net.DialTimeout("unix", filepath.Join(controlDir, supervisorSocket), time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", target, err)
	}
	client := &ProcessClient{conn: conn, enc: json.NewEncoder(conn), events: make(chan ProcessEvent, 256), pending: make(map[uint64]chan error), done: make(chan struct{})}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if err := client.enc.Encode(controlMessage{Type: "attach", ProcessID: target}); err != nil {
		conn.Close()
		return nil, err
	}
	dec := json.NewDecoder(conn)
	var response controlMessage
	if err := dec.Decode(&response); err != nil {
		conn.Close()
		return nil, err
	}
	if response.Type != "attached" || response.Process == nil {
		conn.Close()
		return nil, fmt.Errorf("attach %s: %s", target, response.Error)
	}
	client.snapshot = *response.Process
	client.snapshot.ID = target
	var replay []ProcessEvent
	for {
		if err := dec.Decode(&response); err != nil {
			conn.Close()
			return nil, err
		}
		if response.Type == "ready" {
			break
		}
		if response.Type == "event" && response.Event != nil {
			replay = append(replay, *response.Event)
		}
	}
	client.events = make(chan ProcessEvent, len(replay)+256)
	for _, event := range replay {
		client.events <- event
	}
	_ = conn.SetDeadline(time.Time{})
	go client.read(dec)
	return client, nil
}

func (c *ProcessClient) read(dec *json.Decoder) {
	defer func() {
		c.mu.Lock()
		for id, result := range c.pending {
			delete(c.pending, id)
			result <- fmt.Errorf("daemon connection closed")
			close(result)
		}
		c.mu.Unlock()
		close(c.events)
	}()
	for {
		var message controlMessage
		if dec.Decode(&message) != nil {
			return
		}
		if message.Type == "event" && message.Event != nil {
			select {
			case c.events <- *message.Event:
			case <-c.done:
				return
			}
		} else if message.Type == "input_result" || message.Type == "stop_result" {
			c.mu.Lock()
			result := c.pending[message.ID]
			delete(c.pending, message.ID)
			c.mu.Unlock()
			if result != nil {
				if message.Error != "" {
					result <- fmt.Errorf("%s", message.Error)
				} else {
					result <- nil
				}
				close(result)
			}
		} else if message.Type == "error" {
			select {
			case c.events <- ProcessEvent{Type: "error", Error: message.Error}:
			case <-c.done:
				return
			}
		}
	}
}

// Snapshot 返回 attach 时的远端状态。
func (c *ProcessClient) Snapshot() ProcessSnapshot { return c.snapshot }

// Events 返回 replay 与实时事件流。
func (c *ProcessClient) Events() <-chan ProcessEvent { return c.events }

// Submit 向 daemon Agent 提交一轮用户输入。
func (c *ProcessClient) Submit(text string) error {
	return c.sendControl("input", text)
}

// Stop 请求 daemon Agent 停止当前运行。
func (c *ProcessClient) Stop() error {
	return c.sendControl("stop", "")
}

func (c *ProcessClient) sendControl(kind string, text string) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	result := make(chan error, 1)
	c.pending[id] = result
	_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	err := c.enc.Encode(controlMessage{Type: kind, ID: id, Text: text})
	_ = c.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("%s timed out", kind)
	}
}

// Close 只断开 attached TUI，不取消 daemon Agent。
func (c *ProcessClient) Close() error {
	c.once.Do(func() { close(c.done) })
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.enc.Encode(controlMessage{Type: "detach"})
	return c.conn.Close()
}
