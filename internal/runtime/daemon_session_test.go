// daemon_session_test.go 验证 daemon 交互会话的运行取消和终态事件。
// 调用方：Go test；cancelAwareModel 只等待 context 取消，不访问外部服务。
package runtime

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/agent"
	agentctx "github.com/lzq/5hAgent/internal/context"
	"github.com/lzq/5hAgent/internal/systemd"
)

type cancelAwareModel struct {
	started chan struct{}
	once    sync.Once
}

func (m *cancelAwareModel) wait(ctx context.Context) error {
	m.once.Do(func() { close(m.started) })
	<-ctx.Done()
	return ctx.Err()
}

func (m *cancelAwareModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	return nil, m.wait(ctx)
}

func (m *cancelAwareModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, m.wait(ctx)
}

func (m *cancelAwareModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return m, nil
}

func TestDaemonSessionStopCancelsActiveRun(t *testing.T) {
	model := &cancelAwareModel{started: make(chan struct{})}
	ag, err := agent.NewAgent(model, nil, &agent.Config{
		Name:                "test-agent",
		RepeatToolLimit:     5,
		ContextAutoCompress: false,
		ProjectDataDir:      t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	mgr := agentctx.NewManager()
	messageCtx, err := mgr.CreateContext("")
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	ag.SetCtxManager(mgr)
	rt := &Runtime{
		Agent:      ag,
		CtxManager: mgr,
		MessageCtx: messageCtx,
		ProjectDir: t.TempDir(),
		ModelRef:   "test/model",
		SessionID:  "test-session",
	}
	session := NewDaemonSession(context.Background(), rt)
	_, events, detach := session.Attach()
	defer detach()

	if err := session.Submit("hello"); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("model did not start")
	}
	if err := session.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type != "system" || !strings.Contains(event.Text, "stopped current run") {
				continue
			}
			if state := session.Snapshot().State; state == systemd.ProcessRunning {
				t.Fatalf("Snapshot().State = %s, want non-running", state)
			}
			return
		case <-timer.C:
			t.Fatal("stopped current run event was not published")
		}
	}
}
