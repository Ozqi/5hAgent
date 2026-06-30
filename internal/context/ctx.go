// ctx.go - 消息上下文管理
// 功能：Context 创建/克隆、消息存储、LLM 压缩（当消息数超 MaxMessages 时）
// 主要类型：Context, Manager, CompressResult
// 导出函数：NewManager, CreateContext, CloneContext, GetMessages, AddMessage, Clear, Compress, ShouldCompress, LMCompress, ManualCompress
package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/ipctypes"
	"github.com/lzq/5hAgent/internal/utils"
)

const (
	// MaxMessages 最大消息数，超过后触发压缩
	MaxMessages = 50
	// KeepRecentMessages 压缩后保留的最近消息数
	KeepRecentMessages = 30
)

// Context Agent 运行上下文
// 每个 Agent 实例拥有独立的 Context
type Context struct {
	messages []*schema.Message
	Session  *Session // 关联的持久化会话（可选）
	meta     ContextMeta
}

// Context Manager 负责创建、克隆、管理多个 Context 实例
// 同时管理 Session 持久化
type Manager struct {
	store    *Store // 会话存储
	autobind bool   // CreateContext 是否自动绑定 session
}

type toolRuntimeKey struct{}

// IPC 是 Agent Systemd 暴露给系统工具的最小进程通信接口。
type IPC interface {
	Send(msg ipctypes.Message) error
	Recv(pid string) ([]ipctypes.Message, error)
}

type ToolRuntime struct {
	Manager   *Manager
	Context   *Context
	ProcessID string
	IPC       IPC
}

// WithToolRuntime 以 merge 模式注入消息上下文，可与 WithSystemRuntime 任意顺序组合。
func WithToolRuntime(ctx context.Context, mgr *Manager, msgCtx *Context) context.Context {
	rt, _ := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	rt.Manager = mgr
	rt.Context = msgCtx
	return context.WithValue(ctx, toolRuntimeKey{}, rt)
}

// WithSystemRuntime 以 merge 模式注入进程身份和 IPC，可与 WithToolRuntime 任意顺序组合。
func WithSystemRuntime(ctx context.Context, mgr *Manager, msgCtx *Context, processID string, ipc IPC) context.Context {
	rt, _ := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	rt.Manager = mgr
	rt.Context = msgCtx
	rt.ProcessID = processID
	rt.IPC = ipc
	return context.WithValue(ctx, toolRuntimeKey{}, rt)
}

func ToolRuntimeFrom(ctx context.Context) (ToolRuntime, bool) {
	rt, ok := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	return rt, ok && rt.Manager != nil && rt.Context != nil
}

func SystemRuntimeFrom(ctx context.Context) (ToolRuntime, bool) {
	rt, ok := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	return rt, ok && rt.ProcessID != "" && rt.IPC != nil
}

type CompressResult struct {
	Before      int
	After       int
	ArchivePath string
}

// ContextMeta 保存 Context 的运行时元数据。
type ContextMeta struct {
	Pinned []ContextRange
	Audit  []ContextEvent
}

// ContextRange 表示包含首尾下标的消息范围。
type ContextRange struct {
	Start  int
	End    int
	Reason string
}

// ContextEvent 记录一次上下文管理操作。
type ContextEvent struct {
	Op          string
	Range       ContextRange
	BeforeCount int
	AfterCount  int
	ArchivePath string
	CreatedAt   time.Time
}

// ContextInspect 是当前上下文的结构化视图。
type ContextInspect struct {
	MessageCount    int
	EstimatedChars  int
	ProtectedRanges []ContextRange
	Pinned          []ContextRange
	Messages        []MessageInspect
}

// MessageInspect 是单条消息的结构化索引信息。
type MessageInspect struct {
	Index   int
	Role    string
	Chars   int
	Preview string
	Flags   []string
}

// NewManager 创建新的上下文管理器
// 参数:
//   - sessionDir: 会话存储目录（可选，为空则不启用持久化）
//
// 返回: Manager 实例
func NewManager(sessionDir ...string) *Manager {
	var store *Store
	if len(sessionDir) > 0 && sessionDir[0] != "" {
		store, _ = NewStore(sessionDir[0])
	}
	return &Manager{store: store, autobind: store != nil}
}

// NewMemoryManagerWithStore 创建默认内存 Context、但允许显式绑定 Session 的 manager。
func NewMemoryManagerWithStore(sessionDir string) *Manager {
	store, _ := NewStore(sessionDir)
	return &Manager{store: store, autobind: false}
}

// NewManagerWithStore 创建带有持久化存储的上下文管理器
// 参数:
//   - store: 已初始化的会话存储
//
// 返回: Manager 实例
func NewManagerWithStore(store *Store) *Manager {
	return &Manager{store: store, autobind: store != nil}
}

// CreateContext 创建新的 Context
// 参数:
//   - sessionID: 会话 ID（可选，为空则创建新会话）
//
// 返回: Context 实例和可能的错误
// 功能: 创建一个 Context，可选择关联到指定会话
func (m *Manager) CreateContext(sessionID string) (*Context, error) {
	ctx := &Context{
		messages: make([]*schema.Message, 0),
	}

	if m.store != nil && m.autobind {
		session, err := m.store.GetOrCreate(sessionID)
		if err != nil {
			return nil, err
		}
		ctx.Session = session

		// 从 Session 加载已有消息
		messages, err := m.store.LoadMessages(session)
		if err == nil && len(messages) > 0 {
			ctx.messages = messages
		}
	}

	return ctx, nil
}

// GetSessionID 返回 Context 关联的 Session ID
func (m *Manager) GetSessionID(ctx *Context) string {
	if ctx.Session != nil {
		return ctx.Session.ID
	}
	return ""
}

// GetLatestSessionID 返回最新会话的 ID（按更新时间倒序）
func (m *Manager) GetLatestSessionID() (string, error) {
	if m.store == nil {
		return "", nil
	}
	return m.store.GetLatestID()
}

// GetStore 返回关联的 Store
func (m *Manager) GetStore() *Store {
	return m.store
}

// GetSessionTitle 返回 Context 关联的 Session 标题
func (m *Manager) GetSessionTitle(ctx *Context) string {
	if ctx.Session == nil {
		return ""
	}
	return ctx.Session.Title
}

// BindSession 将内存 Context 显式绑定到持久化 Session。
// 参数：ctx 是当前内存上下文；sessionID 为空时创建新 session。
// 调用层级：sys.session.create 工具 -> BindSession -> Store.GetOrCreate/ReplaceMessages。
// 步骤：获取或创建 session -> 绑定到 ctx -> 把当前 messages 写入 session。
func (m *Manager) BindSession(ctx *Context, sessionID string) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("context is nil")
	}
	if m.store == nil {
		return "", fmt.Errorf("session store not initialized")
	}
	session, err := m.store.GetOrCreate(sessionID)
	if err != nil {
		return "", err
	}
	ctx.Session = session
	if err := m.store.ReplaceMessages(session, ctx.messages); err != nil {
		return "", fmt.Errorf("persist session: %w", err)
	}
	return session.ID, nil
}

// SaveSession 将当前 Context 写入已绑定的 Session。
// 参数：ctx 必须已经通过 BindSession 或 CreateContext 绑定 Session。
// 调用层级：sys.session.save 工具 -> SaveSession -> Store.ReplaceMessages。
// 步骤：校验绑定 -> 重写 session messages。
func (m *Manager) SaveSession(ctx *Context) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if m.store == nil {
		return fmt.Errorf("session store not initialized")
	}
	if ctx.Session == nil {
		return fmt.Errorf("context has no bound session")
	}
	return m.store.ReplaceMessages(ctx.Session, ctx.messages)
}

// DropSession 解除 Context 和 Session 的绑定。
// 参数：ctx 是当前上下文。
// 调用层级：sys.session.drop 工具 -> DropSession。
// 步骤：只清空内存绑定，不删除 session 文件，不清空 messages。
func (m *Manager) DropSession(ctx *Context) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	ctx.Session = nil
	return nil
}

// CloneContext 克隆 Context（用于 sub-agent）
func (m *Manager) CloneContext(parent *Context) (*Context, error) {
	cloned := &Context{
		messages: make([]*schema.Message, len(parent.messages)),
	}
	copy(cloned.messages, parent.messages)
	return cloned, nil
}

// GetMessages 获取 Context 中的所有消息
// 参数:
//   - ctx: Context 实例
//
// 返回: 消息列表和可能的错误
func (m *Manager) GetMessages(ctx *Context) ([]*schema.Message, error) {
	return ctx.messages, nil
}

// Inspect 返回当前上下文的结构化视图，不返回完整消息内容。
func (m *Manager) Inspect(ctx *Context) (*ContextInspect, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}

	inspect := &ContextInspect{
		MessageCount: len(ctx.messages),
		Pinned:       copyRanges(ctx.meta.Pinned),
		Messages:     make([]MessageInspect, 0, len(ctx.messages)),
	}

	protected := make([]bool, len(ctx.messages))
	for i, msg := range ctx.messages {
		if msg.Role == schema.System {
			protected[i] = true
		}
	}
	for _, idx := range recentHistoryIndexes(ctx.messages) {
		protected[idx] = true
	}
	inspect.ProtectedRanges = boolRanges(protected, "protected")

	for i, msg := range ctx.messages {
		chars := len(msg.Content)
		inspect.EstimatedChars += chars
		flags := make([]string, 0, 3)
		if protected[i] {
			flags = append(flags, "protected")
		}
		if rangeContainsAny(ctx.meta.Pinned, i, i) {
			flags = append(flags, "pinned")
		}
		if len(flags) == 0 {
			flags = append(flags, "compressible")
		}
		inspect.Messages = append(inspect.Messages, MessageInspect{
			Index:   i,
			Role:    string(msg.Role),
			Chars:   chars,
			Preview: previewContent(msg.Content, 80),
			Flags:   flags,
		})
	}

	return inspect, nil
}

// PinRange 标记消息范围，避免后续压缩或改写。
func (m *Manager) PinRange(ctx *Context, r ContextRange) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if err := validateRangeBounds(len(ctx.messages), r); err != nil {
		return err
	}
	ctx.meta.Pinned = append(ctx.meta.Pinned, r)
	ctx.meta.Audit = append(ctx.meta.Audit, ContextEvent{
		Op:          "pin",
		Range:       r,
		BeforeCount: len(ctx.messages),
		AfterCount:  len(ctx.messages),
		CreatedAt:   time.Now().UTC(),
	})
	return nil
}

// Audit 返回上下文管理操作记录。
func (m *Manager) Audit(ctx *Context) []ContextEvent {
	if ctx == nil || len(ctx.meta.Audit) == 0 {
		return nil
	}
	events := make([]ContextEvent, len(ctx.meta.Audit))
	copy(events, ctx.meta.Audit)
	return events
}

// ValidateEditableRange 检查指定范围是否允许被压缩或替换。
func (m *Manager) ValidateEditableRange(ctx *Context, r ContextRange) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if err := validateRangeBounds(len(ctx.messages), r); err != nil {
		return err
	}
	if rangeContainsAny(ctx.meta.Pinned, r.Start, r.End) {
		return fmt.Errorf("range %d..%d contains pinned messages", r.Start, r.End)
	}
	for i := r.Start; i <= r.End; i++ {
		if ctx.messages[i].Role == schema.System {
			return fmt.Errorf("range %d..%d contains protected system message at %d", r.Start, r.End, i)
		}
	}
	for _, idx := range recentHistoryIndexes(ctx.messages) {
		if idx >= r.Start && idx <= r.End {
			return fmt.Errorf("range %d..%d contains recent message at %d", r.Start, r.End, idx)
		}
	}
	return nil
}

// AddMessage 添加消息到 Context
// 参数:
//   - ctx: Context 实例
//   - msg: 要添加的消息
//
// 返回: 可能的错误
func (m *Manager) AddMessage(ctx *Context, msg *schema.Message) error {
	ctx.messages = append(ctx.messages, msg)

	// 持久化到 Session
	if m.store != nil && ctx.Session != nil {
		if err := m.store.Append(ctx.Session, msg); err != nil {
			return fmt.Errorf("persist message: %w", err)
		}
	}

	return nil
}

// ReplaceMessages 替换上下文消息，并同步持久化 session。
func (m *Manager) ReplaceMessages(ctx *Context, messages []*schema.Message) error {
	ctx.messages = messages
	if m.store != nil && ctx.Session != nil {
		if err := m.store.ReplaceMessages(ctx.Session, messages); err != nil {
			return fmt.Errorf("persist replaced messages: %w", err)
		}
	}
	return nil
}

// Clear 清空 Context
// 参数:
//   - ctx: Context 实例
//
// 返回: 可能的错误
func (m *Manager) Clear(ctx *Context) error {
	ctx.messages = make([]*schema.Message, 0)
	return nil
}

// ListSessions 列出所有会话
// 返回: Session 列表和可能的错误
func (m *Manager) ListSessions() ([]*Session, error) {
	if m.store == nil {
		return nil, nil
	}
	return m.store.List()
}

// SwitchSession 切换到指定会话
// 参数:
//   - ctx: Context 实例
//   - sessionID: 目标会话 ID
//
// 返回: 新的 Context 实例和可能的错误
func (m *Manager) SwitchSession(ctx *Context, sessionID string) (*Context, error) {
	if m.store == nil {
		return nil, fmt.Errorf("session store not initialized")
	}

	newCtx := &Context{
		messages: make([]*schema.Message, 0),
	}

	session, err := m.store.GetOrCreate(sessionID)
	if err != nil {
		return nil, err
	}

	newCtx.Session = session

	messages, err := m.store.LoadMessages(session)
	if err == nil {
		newCtx.messages = messages
	}

	return newCtx, nil
}

// Compress 压缩上下文（保留最近的消息）
// 参数:
//   - ctx: Context 实例
//
// 返回: 压缩前的消息数、压缩后的消息数、可能的错误
// 功能: 当消息数超过 MaxMessages 时，只保留最近的 KeepRecentMessages 条消息
func (m *Manager) Compress(ctx *Context) (int, int, error) {
	beforeCount := len(ctx.messages)

	if beforeCount <= MaxMessages {
		return beforeCount, beforeCount, nil
	}

	// 保留最近的消息
	keepStart := beforeCount - KeepRecentMessages
	compressed := ctx.messages[keepStart:]
	if err := m.ReplaceMessages(ctx, compressed); err != nil {
		return beforeCount, beforeCount, err
	}

	afterCount := len(ctx.messages)
	return beforeCount, afterCount, nil
}

// ShouldCompress 检查是否需要压缩
func (m *Manager) ShouldCompress(ctx *Context) bool {
	return len(ctx.messages) > MaxMessages
}

// LMCompress 使用 LLM 压缩上下文
// 将对话历史格式化后喂给 LLM，用摘要替换旧消息，保留最近 KeepRecentMessages 条。
// promptDir: prompt 文件目录（用于加载 compress.md）
func (m *Manager) LMCompress(goCtx context.Context, ctx *Context, llm model.ToolCallingChatModel, promptDir string) (int, int, error) {
	before := len(ctx.messages)
	if before <= MaxMessages {
		return before, before, nil
	}

	compressPrompt, err := utils.Load(promptDir, "compress")
	if err != nil {
		return m.Compress(ctx)
	}

	compressed, _, _, err := m.compressWithPrompt(goCtx, ctx, llm, compressPrompt)
	if err != nil {
		return m.Compress(ctx)
	}
	if err := m.ReplaceMessages(ctx, compressed); err != nil {
		return before, before, err
	}

	return before, len(ctx.messages), nil
}

func (m *Manager) ManualCompress(goCtx context.Context, ctx *Context, llm model.ToolCallingChatModel, promptDir string, archiveDir string) (*CompressResult, error) {
	before := len(ctx.messages)
	compressPrompt, err := utils.Load(promptDir, "compress")
	if err != nil {
		return nil, fmt.Errorf("load compress prompt: %w", err)
	}

	compressed, summary, archived, err := m.compressWithPrompt(goCtx, ctx, llm, compressPrompt)
	if err != nil {
		return nil, err
	}
	if len(archived) == 0 {
		return &CompressResult{Before: before, After: before}, nil
	}

	archivePath, err := writeCompressArchive(archiveDir, summary, archived)
	if err != nil {
		return nil, err
	}

	if err := m.ReplaceMessages(ctx, compressed); err != nil {
		return nil, err
	}
	return &CompressResult{Before: before, After: len(ctx.messages), ArchivePath: archivePath}, nil
}

func (m *Manager) compressWithPrompt(goCtx context.Context, ctx *Context, llm model.ToolCallingChatModel, compressPrompt string) ([]*schema.Message, string, []*schema.Message, error) {
	if llm == nil {
		return nil, "", nil, fmt.Errorf("compression model is required")
	}
	systemMsgs, historyMsgs := splitMessages(ctx.messages)
	compressEnd := len(historyMsgs) - KeepRecentMessages
	if compressEnd <= 0 {
		return ctx.messages, "", nil, nil
	}
	toCompress := historyMsgs[:compressEnd]
	toKeep := historyMsgs[compressEnd:]

	resp, err := llm.Generate(goCtx, []*schema.Message{{Role: schema.User, Content: compressPrompt + formatMessagesForCompression(toCompress)}})
	if err != nil {
		return nil, "", nil, err
	}

	summaryContent := "[对话历史摘要]\n" + resp.Content
	compressed := append([]*schema.Message{}, systemMsgs...)
	compressed = append(compressed, &schema.Message{Role: schema.System, Content: summaryContent})
	compressed = append(compressed, toKeep...)
	return compressed, resp.Content, toCompress, nil
}

func splitMessages(messages []*schema.Message) (systemMsgs []*schema.Message, historyMsgs []*schema.Message) {
	for _, msg := range messages {
		if msg.Role == schema.System {
			systemMsgs = append(systemMsgs, msg)
			continue
		}
		historyMsgs = append(historyMsgs, msg)
	}
	return systemMsgs, historyMsgs
}

func formatMessagesForCompression(messages []*schema.Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, msg.Content))
	}
	return sb.String()
}

func validateRangeBounds(messageCount int, r ContextRange) error {
	if messageCount == 0 {
		return fmt.Errorf("context has no messages")
	}
	if r.Start < 0 || r.End < 0 || r.Start > r.End || r.End >= messageCount {
		return fmt.Errorf("invalid range %d..%d for %d messages", r.Start, r.End, messageCount)
	}
	return nil
}

func recentHistoryIndexes(messages []*schema.Message) []int {
	indexes := make([]int, 0, KeepRecentMessages)
	for i := len(messages) - 1; i >= 0 && len(indexes) < KeepRecentMessages; i-- {
		if messages[i].Role == schema.System {
			continue
		}
		indexes = append(indexes, i)
	}
	return indexes
}

func rangeContainsAny(ranges []ContextRange, start, end int) bool {
	for _, r := range ranges {
		if start <= r.End && end >= r.Start {
			return true
		}
	}
	return false
}

func copyRanges(ranges []ContextRange) []ContextRange {
	if len(ranges) == 0 {
		return nil
	}
	copied := make([]ContextRange, len(ranges))
	copy(copied, ranges)
	return copied
}

func boolRanges(values []bool, reason string) []ContextRange {
	ranges := make([]ContextRange, 0)
	for i := 0; i < len(values); i++ {
		if !values[i] {
			continue
		}
		start := i
		for i+1 < len(values) && values[i+1] {
			i++
		}
		ranges = append(ranges, ContextRange{Start: start, End: i, Reason: reason})
	}
	return ranges
}

func previewContent(content string, maxRunes int) string {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func writeCompressArchive(archiveDir string, summary string, archived []*schema.Message) (string, error) {
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", fmt.Errorf("create archive dir: %w", err)
	}
	path := filepath.Join(archiveDir, time.Now().UTC().Format("20060102-150405")+".md")
	var b strings.Builder
	b.WriteString("# Compressed Context Archive\n\n")
	b.WriteString("## Summary\n\n")
	b.WriteString(summary)
	b.WriteString("\n\n## Archived Messages\n\n")
	for _, msg := range archived {
		b.WriteString(fmt.Sprintf("- role: %s\n", msg.Role))
		b.WriteString(fmt.Sprintf("  content: %s\n", strings.ReplaceAll(msg.Content, "\n", " ")))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write archive: %w", err)
	}
	return path, nil
}
