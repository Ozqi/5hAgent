// Package context 管理 Agent 消息上下文、会话持久化、上下文审计和压缩。
package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ozqi/walle/internal/utils"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	// MaxMessages 最大消息数，超过后触发压缩
	MaxMessages = 50
	// KeepRecentMessages 压缩后保留的最近消息数
	KeepRecentMessages = 30
)

// Context 保存一个会话或 AgentProcess 的消息和运行时元数据。
// mu 同时保护 messages、Session 引用和仅驻留内存的 meta。
type Context struct {
	mu       sync.RWMutex
	messages []*schema.Message
	Session  *Session // 关联的持久化会话（可选）
	meta     ContextMeta
}

// Manager 创建和管理多个 Context，并按配置将消息同步到可选的 Session Store。
// autobind 只控制 CreateContext 是否自动绑定 Session，不影响显式的 Session 操作。
type Manager struct {
	store    *Store // 会话存储
	autobind bool   // CreateContext 是否自动绑定 session
}

type toolRuntimeKey struct{}

// ToolRuntime 保存工具调用期间可访问的消息管理器和当前 Context。
type ToolRuntime struct {
	Manager *Manager
	Context *Context
}

// WithToolRuntime 把当前消息运行时注入标准 context，供 context 工具读取。
func WithToolRuntime(ctx context.Context, mgr *Manager, msgCtx *Context) context.Context {
	rt, _ := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	rt.Manager = mgr
	rt.Context = msgCtx
	return context.WithValue(ctx, toolRuntimeKey{}, rt)
}

// ToolRuntimeFrom 从标准 context 读取完整的消息运行时。
func ToolRuntimeFrom(ctx context.Context) (ToolRuntime, bool) {
	rt, ok := ctx.Value(toolRuntimeKey{}).(ToolRuntime)
	return rt, ok && rt.Manager != nil && rt.Context != nil
}

// CompressResult 描述一次手动压缩前后的消息数和归档文件路径。
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
			ctx.messages = copyMessageSlice(messages)
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
	return copyMessageSlice(ctx.messages), nil
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

	// 1. system 消息和最近历史属于自动保护范围，再叠加用户显式 pin 标记。
	protected := make([]bool, len(ctx.messages))
	// 2. 只生成长度、预览和标记，避免工具直接暴露完整上下文。
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

// AddMessage 先更新内存消息，再把新增消息追加到关联的 Session。
// Store 写入不在 Context 锁内执行；写入失败时内存消息已经生效。
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

	// 解锁后执行磁盘 I/O，避免阻塞同一 Context 的读取。
	if m.store != nil && session != nil {
		if err := m.store.Append(session, msg); err != nil {
			return fmt.Errorf("persist message: %w", err)
		}
	}

	return nil
}

// EditMessage 修改一条普通对话消息，并同步持久化 Session。
// system、包含 ToolCall 或已 pinned 的消息不可编辑；持久化失败时内存修改已经生效。
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

	// 1. 持锁校验目标消息，更新内存并生成完整消息快照。
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
	messages := copyMessageSlice(ctx.messages)
	session := ctx.Session
	ctx.mu.Unlock()

	// 2. 解锁后替换 Session 文件，避免持锁执行磁盘 I/O。
	if m.store != nil && session != nil {
		if err := m.store.ReplaceMessages(session, messages); err != nil {
			return fmt.Errorf("persist edited message: %w", err)
		}
	}
	return nil
}

// ReplaceMessages 是压缩和批量改写后的统一入口，先替换内存，再同步关联的 Session。
// Store 写入失败时内存消息已经生效。
func (m *Manager) ReplaceMessages(ctx *Context, messages []*schema.Message) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	ctx.mu.Lock()
	ctx.messages = copyMessageSlice(messages)
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
		newCtx.messages = copyMessageSlice(messages)
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

// LMCompress 使用 LLM 摘要旧的非 system 消息，并保留全部 system 消息和最近历史。
// promptDir 用于加载 compress.md；加载或模型调用失败时降级为不生成摘要的 Compress。
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

	// prompt 或模型压缩失败时降级为仅保留 system 和最近历史。
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

// ManualCompress 用 LLM 摘要旧消息，先写 Markdown 归档，再替换当前上下文和 Session。
// 与 LMCompress 不同，手动压缩失败时直接返回错误，不执行 fallback。
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

	// 归档成功后才替换消息，避免压缩内容未落盘便丢失原始历史。
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
	// pinned 当前只约束显式编辑，不参与此处的压缩筛选。
	ctx.mu.RLock()
	messages := copyMessageSlice(ctx.messages)
	ctx.mu.RUnlock()
	systemMsgs, historyMsgs := splitMessages(messages)
	compressEnd := len(historyMsgs) - KeepRecentMessages
	if compressEnd <= 0 {
		return messages, "", nil, nil
	}
	toCompress := historyMsgs[:compressEnd]
	toKeep := historyMsgs[compressEnd:]

	// 模型 I/O 仅接收待归档历史；system 和最近消息不进入摘要请求。
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
		return append(copyMessageSlice(systemMsgs), historyMsgs...)
	}
	keepStart := len(historyMsgs) - KeepRecentMessages
	compressed := copyMessageSlice(systemMsgs)
	compressed = append(compressed, historyMsgs[keepStart:]...)
	return compressed
}

func copyMessageSlice(messages []*schema.Message) []*schema.Message {
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
