// daemon_session.go - daemon 持有的长驻交互 Agent 会话。
// 控制层只通过 systemd.InteractiveProcess 接口提交输入和订阅事件，不接触 Runtime 内部对象。
package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/systemd"
)

// DaemonSession 串行执行用户输入，并缓存 daemon 生命周期内的结构化输出供重连回放。
type DaemonSession struct {
	mu      sync.Mutex
	ctx     context.Context
	runtime *Runtime
	started time.Time
	busy    bool
	seq     uint64
	events  []systemd.ProcessEvent
	subs    map[chan systemd.ProcessEvent]struct{}
}

// NewDaemonSession 为一个 Runtime 创建长驻交互会话。
func NewDaemonSession(ctx context.Context, rt *Runtime) *DaemonSession {
	return &DaemonSession{ctx: ctx, runtime: rt, started: time.Now().UTC(), subs: make(map[chan systemd.ProcessEvent]struct{})}
}

// Snapshot 返回 control socket 使用的只读会话状态。
func (s *DaemonSession) Snapshot() systemd.ProcessSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := systemd.ProcessIdle
	if s.busy {
		state = systemd.ProcessRunning
	}
	return systemd.ProcessSnapshot{
		ID: "interactive", State: state, TaskTitle: "interactive", StartedAt: s.started,
		Workspace: s.runtime.ProjectDir, Model: s.runtime.ModelName, SessionID: s.runtime.SessionID, Interactive: true,
	}
}

// Attach 原子返回历史事件并注册实时订阅者。
func (s *DaemonSession) Attach() ([]systemd.ProcessEvent, <-chan systemd.ProcessEvent, func()) {
	s.mu.Lock()
	history := append([]systemd.ProcessEvent(nil), s.events...)
	ch := make(chan systemd.ProcessEvent, 256)
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return history, ch, func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
}

// Submit 在会话空闲时启动一轮 Agent；socket 断开不会取消这轮执行。
func (s *DaemonSession) Submit(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("input is required")
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return fmt.Errorf("agent is busy")
	}
	s.busy = true
	s.mu.Unlock()
	s.publish(systemd.ProcessEvent{Type: "user", Text: text})
	s.publish(systemd.ProcessEvent{Type: "state", Busy: true})
	go s.run(text)
	return nil
}

func (s *DaemonSession) run(text string) {
	prev := s.runtime.Agent.SetToolEventSink(func(event logger.ToolEvent) {
		s.runtime.RecordToolEvent(event)
		s.publish(systemd.ProcessEvent{
			Type: "tool", Kind: event.Kind, Name: event.Name, Args: event.Args,
			Text: event.Text, Result: event.Result, Error: event.Error,
		})
	})
	_, err := s.runtime.Agent.RunStream(s.ctx, s.runtime.MessageCtx, text,
		func(token string) { s.publish(systemd.ProcessEvent{Type: "assistant", Text: token}) },
		func(token string) { s.publish(systemd.ProcessEvent{Type: "thinking", Text: token}) },
	)
	s.runtime.Agent.SetToolEventSink(prev)
	s.mu.Lock()
	if err != nil {
		s.publishLocked(systemd.ProcessEvent{Type: "error", Error: err.Error()})
	} else {
		s.publishLocked(systemd.ProcessEvent{Type: "done"})
	}
	s.publishLocked(systemd.ProcessEvent{Type: "state"})
	s.busy = false
	s.mu.Unlock()
}

func (s *DaemonSession) publish(event systemd.ProcessEvent) {
	s.mu.Lock()
	s.publishLocked(event)
	s.mu.Unlock()
}

func (s *DaemonSession) publishLocked(event systemd.ProcessEvent) {
	s.seq++
	event.Seq = s.seq
	if len(s.events) > 0 && (event.Type == "assistant" || event.Type == "thinking") && s.events[len(s.events)-1].Type == event.Type {
		s.events[len(s.events)-1].Text += event.Text
		s.events[len(s.events)-1].Seq = event.Seq
	} else {
		s.events = append(s.events, event)
	}
	for ch := range s.subs {
		select {
		case ch <- event:
		default:
			delete(s.subs, ch)
			close(ch)
		}
	}
}
