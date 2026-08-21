// daemon_session.go - daemon 持有的长驻交互 Agent 会话。
// 控制层只通过 systemd.InteractiveProcess 接口提交输入和订阅事件，不接触 Runtime 内部对象。
package runtime

import (
	"context"
	"fmt"
	"github.com/lzq/5hAgent/internal/toolevent"
	"strings"
	"sync"
	"time"

	"github.com/lzq/5hAgent/internal/commands"
	"github.com/lzq/5hAgent/internal/systemd"
)

// DaemonSession 串行执行用户输入，并缓存 daemon 生命周期内的结构化输出供重连回放。
type DaemonSession struct {
	mu        sync.Mutex
	ctx       context.Context
	runtime   *Runtime
	started   time.Time
	busy      bool
	provider  string
	runCancel context.CancelFunc
	seq       uint64
	events    []systemd.ProcessEvent
	subs      map[chan systemd.ProcessEvent]struct{}
}

// NewDaemonSession 为一个 Runtime 创建长驻交互会话。
func NewDaemonSession(ctx context.Context, rt *Runtime) *DaemonSession {
	provider, _, _ := strings.Cut(rt.ModelRef, "/")
	return &DaemonSession{ctx: ctx, runtime: rt, provider: provider, started: time.Now().UTC(), subs: make(map[chan systemd.ProcessEvent]struct{})}
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
		Workspace: s.runtime.ProjectDir, Model: s.runtime.ModelRef, SessionID: s.runtime.SessionID, Interactive: true,
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
	if strings.HasPrefix(text, "/") {
		s.publish(systemd.ProcessEvent{Type: "user", Text: text})
		if text == "/stop" {
			return s.stop()
		}
		if s.handlePickerSlash(text) {
			return nil
		}
		s.publish(systemd.ProcessEvent{Type: "system", Text: s.handleSlash(text)})
		return nil
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return fmt.Errorf("agent is busy")
	}
	runCtx, cancel := context.WithCancel(s.ctx)
	s.busy = true
	s.runCancel = cancel
	s.mu.Unlock()
	s.publish(systemd.ProcessEvent{Type: "user", Text: text})
	s.publish(systemd.ProcessEvent{Type: "state", Busy: true})
	go s.run(runCtx, text)
	return nil
}

// Stop 停止当前 attached daemon 会话正在执行的一轮 Agent。
func (s *DaemonSession) Stop() error {
	return s.stop()
}

func (s *DaemonSession) stop() error {
	s.mu.Lock()
	cancel := s.runCancel
	if !s.busy || cancel == nil {
		s.mu.Unlock()
		s.publish(systemd.ProcessEvent{Type: "system", Text: "no active run"})
		return nil
	}
	s.mu.Unlock()
	cancel()
	return nil
}

func (s *DaemonSession) handlePickerSlash(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 || (fields[0] != "/provider" && fields[0] != "/model") {
		return false
	}
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if busy {
		s.publish(systemd.ProcessEvent{Type: "system", Text: "agent is busy"})
		return true
	}
	if fields[0] == "/provider" {
		if len(fields) == 1 {
			providers := s.runtime.Providers()
			options := make([]string, 0, len(providers))
			for _, provider := range providers {
				options = append(options, provider.Name)
			}
			s.publish(systemd.ProcessEvent{Type: "picker", Kind: "provider", Options: options})
			return true
		}
		if len(fields) != 2 {
			s.publish(systemd.ProcessEvent{Type: "system", Text: "usage: /provider [name]"})
			return true
		}
		s.setProvider(fields[1])
		provider := s.currentProvider()
		if provider == "openai" {
			for _, provider := range s.runtime.Providers() {
				if provider.Name == "openai" && provider.LoggedIn {
					go s.publishModels("openai")
					return true
				}
			}
			loginURL, done, err := s.runtime.StartOpenAILogin(s.ctx)
			if err != nil {
				s.publish(systemd.ProcessEvent{Type: "system", Text: err.Error()})
				return true
			}
			s.publish(systemd.ProcessEvent{Type: "system", Text: "Open this URL to sign in with ChatGPT:\n" + loginURL})
			go func() {
				if err := <-done; err != nil {
					s.publish(systemd.ProcessEvent{Type: "system", Text: "Codex login failed: " + err.Error()})
					return
				}
				s.publish(systemd.ProcessEvent{Type: "system", Text: "Codex login complete"})
				go s.publishModels("openai")
			}()
			return true
		}
		go s.publishModels(provider)
		return true
	}
	if len(fields) == 1 {
		go s.publishModels(s.currentProvider())
		return true
	}
	if len(fields) != 2 {
		s.publish(systemd.ProcessEvent{Type: "system", Text: "usage: /model [name]"})
		return true
	}
	modelRef := fields[1]
	if !strings.Contains(modelRef, "/") {
		modelRef = s.currentProvider() + "/" + modelRef
	}
	go s.switchModel(modelRef)
	return true
}

func (s *DaemonSession) switchModel(modelRef string) {
	result, err := s.runtime.SwitchModel(s.ctx, modelRef)
	if err != nil {
		s.publish(systemd.ProcessEvent{Type: "system", Text: err.Error()})
		return
	}
	provider, _, _ := strings.Cut(result, "/")
	s.setProvider(provider)
	s.publish(systemd.ProcessEvent{Type: "model", Text: result})
	s.publish(systemd.ProcessEvent{Type: "system", Text: "Switched model: " + result})
}

func (s *DaemonSession) currentProvider() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.provider
}

func (s *DaemonSession) setProvider(provider string) {
	s.mu.Lock()
	s.provider = provider
	s.mu.Unlock()
}

func (s *DaemonSession) publishModels(provider string) {
	models, err := s.runtime.ProviderModels(s.ctx, provider)
	if err != nil {
		s.publish(systemd.ProcessEvent{Type: "system", Text: err.Error()})
		return
	}
	s.publish(systemd.ProcessEvent{Type: "picker", Kind: "model", Name: provider, Options: models})
}

func (s *DaemonSession) handleSlash(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	var result string
	var err error
	switch fields[0] {
	case "/skill":
		result, err = commands.HandleSkill(text, s.runtime.Agent.GetSkillManager())
	case "/task":
		result, err = commands.HandleTask(text, s.runtime.TaskList)
	case "/mcp":
		result, err = commands.HandleMCP(text)
	case "/compress":
		result, err = commands.HandleCompress(s.ctx, text, s.runtime.CtxManager, s.runtime.MessageCtx, s.runtime.Agent.GetModel(), s.runtime.PromptDir, "compact")
	case "/run":
		return "/run is not available in attached daemon mode yet"
	case "/stop":
		if err := s.stop(); err != nil {
			return err.Error()
		}
		return ""
	case "/session":
		return fmt.Sprintf("Current session: %s", s.runtime.SessionID)
	default:
		return "unknown slash command"
	}
	if err != nil {
		return err.Error()
	}
	return result
}

func (s *DaemonSession) run(runCtx context.Context, text string) {
	prev := s.runtime.Agent.SetToolEventSink(func(event toolevent.ToolEvent) {
		s.runtime.RecordToolEvent(event)
		s.publish(systemd.ProcessEvent{
			Type: "tool", Kind: event.Kind, Name: event.Name, Args: event.Args,
			Text: event.Text, Result: event.Result, Error: event.Error,
		})
	})
	_, err := s.runtime.Agent.RunStream(runCtx, s.runtime.MessageCtx, text,
		func(token string) { s.publish(systemd.ProcessEvent{Type: "assistant", Text: token}) },
		func(token string) { s.publish(systemd.ProcessEvent{Type: "thinking", Text: token}) },
	)
	s.runtime.Agent.SetToolEventSink(prev)
	s.mu.Lock()
	if runCtx.Err() != nil {
		s.publishLocked(systemd.ProcessEvent{Type: "system", Text: "stopped current run"})
	} else if err != nil {
		s.publishLocked(systemd.ProcessEvent{Type: "error", Error: err.Error()})
	} else {
		s.publishLocked(systemd.ProcessEvent{Type: "done"})
	}
	s.publishLocked(systemd.ProcessEvent{Type: "state"})
	s.busy = false
	s.runCancel = nil
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
