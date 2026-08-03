// tui_remote.go - attached TUI 的 Unix Socket 客户端入口。
package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lzq/5hAgent/internal/systemd"
)

// RemoteClient 是任意 attached TUI 所需的最小 daemon 客户端契约。
type RemoteClient interface {
	Snapshot() systemd.ProcessSnapshot
	Events() <-chan systemd.ProcessEvent
	Submit(string) error
	Close() error
}

// LaunchAttachedTUI 连接 daemon Agent；退出界面只关闭客户端连接。
func LaunchAttachedTUI(ctx context.Context, client RemoteClient) error {
	snapshot := client.Snapshot()
	model := NewAppModel(ctx, nil, snapshot.Model, "", nil, nil, nil, nil, snapshot.SessionID, nil, nil)
	model.remoteSubmit = client.Submit
	model.busy = snapshot.State == systemd.ProcessRunning
	if model.busy {
		model.currentStatus = "attached"
	}
	model.metaCache.Workdir = snapshot.Workspace
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	model.program = p
	go func() {
		for event := range client.Events() {
			p.Send(remoteEventMsg{event: event})
		}
		p.Send(remoteDisconnectedMsg{})
	}()
	_, err := p.Run()
	_ = client.Close()
	return err
}
