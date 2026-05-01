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
}

// Context Manager 负责创建、克隆、管理多个 Context 实例
// 同时管理 Session 持久化
type Manager struct {
	store *Store // 会话存储
}

type CompressResult struct {
	Before      int
	After       int
	ArchivePath string
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
	return &Manager{store: store}
}

// NewManagerWithStore 创建带有持久化存储的上下文管理器
// 参数:
//   - store: 已初始化的会话存储
//
// 返回: Manager 实例
func NewManagerWithStore(store *Store) *Manager {
	return &Manager{store: store}
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

	if m.store != nil {
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

// CloneContext 克隆 Context（用于 sub-agent）
// 参数:
//   - parent: 父 Context
//
// 返回: 新的 Context 实例和可能的错误
// 功能: 克隆父 Context 的消息历史，创建隔离的副本
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

// Clear 清空 Context
// 参数:
//   - ctx: Context 实例
//
// 返回: 可能的错误
func (m *Manager) Clear(ctx *Context) error {
	ctx.messages = make([]*schema.Message, 0)
	return nil
}

// GetStore 返回 Manager 的 Store 实例
func (m *Manager) GetStore() *Store {
	return m.store
}

// GetSessionID 返回 Context 关联的 Session ID
func (m *Manager) GetSessionID(ctx *Context) string {
	if ctx.Session == nil {
		return ""
	}
	return ctx.Session.ID
}

// GetSessionTitle 返回 Context 关联的 Session 标题
func (m *Manager) GetSessionTitle(ctx *Context) string {
	if ctx.Session == nil {
		return ""
	}
	return ctx.Session.Title
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
	ctx.messages = ctx.messages[keepStart:]

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
	ctx.messages = compressed

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

	ctx.messages = compressed
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
