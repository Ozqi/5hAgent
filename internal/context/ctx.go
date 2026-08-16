// ctx.go - 消息上下文管理
// 功能：Context 创建、消息存储、LLM 压缩（当消息数超 MaxMessages 时）
// 主要类型：Context, Manager, CompressResult
// 导出函数：NewManager, CreateContext, GetMessages, AddMessage, Compress, ShouldCompress, LMCompress, ManualCompress
package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
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
	mu       sync.RWMutex
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

type ToolRuntime struct {
	Manager *Manager
	Context *Context
}

func WithToolRuntime(ctx context.Context, mgr *Manager, msgCtx *Context) context.Context {
	rt, _ := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	rt.Manager = mgr
	rt.Context = msgCtx
	return context.WithValue(ctx, toolRuntimeKey{}, rt)
}

func ToolRuntimeFrom(ctx context.Context) (ToolRuntime, bool) {
	rt, ok := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	return rt, ok && rt.Manager != nil && rt.Context != nil
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
			ctx.messages = cloneMessages(messages)
		}
	}

	return ctx, nil
}

// GetSessionID 返回 Context 关联的 Session ID
func (m *Manager) GetSessionID(ctx *Context) string {
	if ctx == nil {
		return ""
	}
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
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

// GetSessionTitle 返回 Context 关联的 Session 标题
func (m *Manager) GetSessionTitle(ctx *Context) string {
	if ctx == nil {
		return ""
	}
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	if ctx.Session == nil {
		return ""
	}
	return ctx.Session.Title
}

// GetMessages 获取 Context 中的所有消息
// 参数:
//   - ctx: Context 实例
//
// 返回: 消息列表和可能的错误
func (m *Manager) GetMessages(ctx *Context) ([]*schema.Message, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return cloneMessages(ctx.messages), nil
}

// Inspect 返回当前上下文的结构化视图，不返回完整消息内容。
func (m *Manager) Inspect(ctx *Context) (*ContextInspect, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

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
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
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
	if ctx == nil {
		return nil
	}
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	if len(ctx.meta.Audit) == 0 {
		return nil
	}
	events := make([]ContextEvent, len(ctx.meta.Audit))
	copy(events, ctx.meta.Audit)
	return events
}

// AddMessage 添加消息到 Context
// 参数:
//   - ctx: Context 实例
//   - msg: 要添加的消息
//
// 返回: 可能的错误
func (m *Manager) AddMessage(ctx *Context, msg *schema.Message) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	ctx.mu.Lock()
	ctx.messages = append(ctx.messages, msg)
	session := ctx.Session
	ctx.mu.Unlock()

	// 持久化到 Session
	if m.store != nil && session != nil {
		if err := m.store.Append(session, msg); err != nil {
			return fmt.Errorf("persist message: %w", err)
		}
	}

	return nil
}

// EditMessage 修改一条普通对话消息，并同步持久化 session。
func (m *Manager) EditMessage(ctx *Context, index int, content string, reason string) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if content == "" {
		return fmt.Errorf("content is required")
	}
	if reason == "" {
		return fmt.Errorf("reason is required")
	}

	ctx.mu.Lock()
	if index < 0 || index >= len(ctx.messages) {
		ctx.mu.Unlock()
		return fmt.Errorf("invalid message index %d for %d messages", index, len(ctx.messages))
	}
	if rangeContainsAny(ctx.meta.Pinned, index, index) {
		ctx.mu.Unlock()
		return fmt.Errorf("message %d is pinned", index)
	}
	msg := ctx.messages[index]
	if msg == nil {
		ctx.mu.Unlock()
		return fmt.Errorf("message %d is nil", index)
	}
	if msg.Role != schema.User && msg.Role != schema.Assistant {
		ctx.mu.Unlock()
		return fmt.Errorf("message %d with role %q cannot be edited", index, msg.Role)
	}
	if len(msg.ToolCalls) > 0 {
		ctx.mu.Unlock()
		return fmt.Errorf("assistant message %d contains tool calls", index)
	}

	edited := *msg
	edited.Content = content
	ctx.messages[index] = &edited
	ctx.meta.Audit = append(ctx.meta.Audit, ContextEvent{
		Op:          "edit",
		Range:       ContextRange{Start: index, End: index, Reason: reason},
		BeforeCount: len(ctx.messages),
		AfterCount:  len(ctx.messages),
		CreatedAt:   time.Now().UTC(),
	})
	messages := cloneMessages(ctx.messages)
	session := ctx.Session
	ctx.mu.Unlock()

	if m.store != nil && session != nil {
		if err := m.store.ReplaceMessages(session, messages); err != nil {
			return fmt.Errorf("persist edited message: %w", err)
		}
	}
	return nil
}

// ReplaceMessages 替换上下文消息，并同步持久化 session。
func (m *Manager) ReplaceMessages(ctx *Context, messages []*schema.Message) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	ctx.mu.Lock()
	ctx.messages = cloneMessages(messages)
	session := ctx.Session
	ctx.mu.Unlock()
	if m.store != nil && session != nil {
		if err := m.store.ReplaceMessages(session, messages); err != nil {
			return fmt.Errorf("persist replaced messages: %w", err)
		}
	}
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
		newCtx.messages = cloneMessages(messages)
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
	if ctx == nil {
		return 0, 0, fmt.Errorf("context is nil")
	}
	ctx.mu.RLock()
	beforeCount := len(ctx.messages)

	if beforeCount <= MaxMessages {
		ctx.mu.RUnlock()
		return beforeCount, beforeCount, nil
	}

	compressed := compressFallbackMessages(ctx.messages)
	ctx.mu.RUnlock()
	if err := m.ReplaceMessages(ctx, compressed); err != nil {
		return beforeCount, beforeCount, err
	}

	return beforeCount, len(compressed), nil
}

// ShouldCompress 检查是否需要压缩
func (m *Manager) ShouldCompress(ctx *Context) bool {
	if ctx == nil {
		return false
	}
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return len(ctx.messages) > MaxMessages
}

// LMCompress 使用 LLM 压缩上下文
// 将对话历史格式化后喂给 LLM，用摘要替换旧消息，保留最近 KeepRecentMessages 条。
// promptDir: prompt 文件目录（用于加载 compress.md）
func (m *Manager) LMCompress(goCtx context.Context, ctx *Context, llm model.ToolCallingChatModel, promptDir string) (int, int, error) {
	if ctx == nil {
		return 0, 0, fmt.Errorf("context is nil")
	}
	ctx.mu.RLock()
	before := len(ctx.messages)
	ctx.mu.RUnlock()
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
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	ctx.mu.RLock()
	before := len(ctx.messages)
	ctx.mu.RUnlock()
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
	// 压缩边界：system 消息全部保留；较旧 history 汇总成一条摘要；最近消息原样保留。
	ctx.mu.RLock()
	messages := cloneMessages(ctx.messages)
	ctx.mu.RUnlock()
	systemMsgs, historyMsgs := splitMessages(messages)
	compressEnd := len(historyMsgs) - KeepRecentMessages
	if compressEnd <= 0 {
		return messages, "", nil, nil
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

func compressFallbackMessages(messages []*schema.Message) []*schema.Message {
	systemMsgs, historyMsgs := splitMessages(messages)
	if len(historyMsgs) <= KeepRecentMessages {
		return append(cloneMessages(systemMsgs), historyMsgs...)
	}
	keepStart := len(historyMsgs) - KeepRecentMessages
	compressed := cloneMessages(systemMsgs)
	compressed = append(compressed, historyMsgs[keepStart:]...)
	return compressed
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]*schema.Message, len(messages))
	copy(cloned, messages)
	return cloned
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
