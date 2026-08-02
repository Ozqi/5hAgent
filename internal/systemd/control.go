// control.go - Agent Systemd 本地只读控制通道。
// daemon 通过 Unix socket 暴露运行中 AgentProcess；CLI 用同一接口实现 ps、attach 和 Tab 补全。
package systemd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProcessSnapshot 是跨进程查询使用的只读 AgentProcess 快照。
type ProcessSnapshot struct {
	ID          string       `json:"id"`
	State       ProcessState `json:"state"`
	TaskID      string       `json:"task_id,omitempty"`
	TaskTitle   string       `json:"task_title,omitempty"`
	StartedAt   time.Time    `json:"started_at"`
	Workspace   string       `json:"workspace"`
	WorkLogPath string       `json:"worklog_path,omitempty"`
}

// Processes 返回当前运行中进程的副本，不外泄进程表中的可变指针。
func (s *AgentSystemd) Processes(workspace string) []ProcessSnapshot {
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

// ControlServer 向同一用户的 CLI 暴露 daemon 进程快照。
type ControlServer struct {
	listener net.Listener
	path     string
}

// StartControlServer 在 controlDir 创建当前 daemon 的 Unix socket。
func StartControlServer(ctx context.Context, controlDir string, sys *AgentSystemd, workspace string) (*ControlServer, error) {
	if sys == nil {
		return nil, fmt.Errorf("agent systemd is required")
	}
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return nil, fmt.Errorf("create daemon control dir: %w", err)
	}
	path := filepath.Join(controlDir, fmt.Sprintf("daemon-%d.sock", os.Getpid()))
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen daemon control socket: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	server := &ControlServer{listener: listener, path: path}
	go server.serve(ctx, sys, workspace)
	return server, nil
}

func (s *ControlServer) serve(ctx context.Context, sys *AgentSystemd, workspace string) {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		_ = json.NewEncoder(conn).Encode(sys.Processes(workspace))
		_ = conn.Close()
	}
}

// Close 关闭控制 socket 并删除 socket 文件。
func (s *ControlServer) Close() error {
	if s == nil {
		return nil
	}
	err := s.listener.Close()
	_ = os.Remove(s.path)
	return err
}

// ListProcesses 聚合 controlDir 中所有存活 daemon 的运行进程。
// 返回 target 形如 daemon-<pid>/agent-<n>，避免多个 daemon 的本地进程号冲突。
func ListProcesses(controlDir string) ([]ProcessSnapshot, error) {
	sockets, err := filepath.Glob(filepath.Join(controlDir, "daemon-*.sock"))
	if err != nil {
		return nil, fmt.Errorf("list daemon sockets: %w", err)
	}
	var processes []ProcessSnapshot
	for _, socket := range sockets {
		conn, err := net.DialTimeout("unix", socket, 200*time.Millisecond)
		if err != nil {
			_ = os.Remove(socket) // daemon 已退出时清理遗留 socket。
			continue
		}
		var listed []ProcessSnapshot
		err = json.NewDecoder(conn).Decode(&listed)
		_ = conn.Close()
		if err != nil {
			continue
		}
		daemon := strings.TrimSuffix(filepath.Base(socket), ".sock")
		for i := range listed {
			listed[i].ID = daemon + "/" + listed[i].ID
		}
		processes = append(processes, listed...)
	}
	sort.Slice(processes, func(i, j int) bool { return processes[i].StartedAt.Before(processes[j].StartedAt) })
	return processes, nil
}
