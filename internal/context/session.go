// session.go - 消息持久化：Session 和 Store
// 功能：会话存储管理，支持创建/恢复/列出/删除会话
// 主要类型：Session, Store
package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

// Session 代表一次完整的对话会话
type Session struct {
	ID        string            // 唯一标识符
	Title     string            // 会话标题（从第一条用户消息生成）
	CreatedAt time.Time         // 创建时间
	UpdatedAt time.Time         // 最后更新时间
	messages  []*schema.Message // 消息历史（内存缓存）
	filePath  string            // JSONL 文件路径
	dirty     bool              // 是否有未保存的修改
}

// Store 管理多个 Session 的持久化存储
type Store struct {
	dir   string              // 存储目录
	cache map[string]*Session // 内存缓存
}

// sessionFileEntry JSONL 文件中的单条记录
type sessionFileEntry struct {
	Type       string            `json:"type,omitempty"`         // "session" 表示会话头
	ID         string            `json:"id,omitempty"`           // 会话 ID
	Title      string            `json:"title,omitempty"`        // 会话标题
	CreatedAt  string            `json:"created_at,omitempty"`   // 创建时间
	UpdatedAt  string            `json:"updated_at,omitempty"`   // 更新时间
	Role       string            `json:"role,omitempty"`         // 消息角色
	Content    string            `json:"content,omitempty"`      // 消息内容
	ToolCalls  []schema.ToolCall `json:"tool_calls,omitempty"`   // assistant 发起的工具调用
	ToolCallID string            `json:"tool_call_id,omitempty"` // tool result 对应的调用 ID
	ToolName   string            `json:"tool_name,omitempty"`    // tool result 对应的工具名
}

// NewStore 创建或打开会话存储
// 参数:
//   - dir: 存储目录路径
//
// 返回: Store 实例和可能的错误
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	return &Store{
		dir:   dir,
		cache: make(map[string]*Session),
	}, nil
}

// GetOrCreate 获取或创建 Session
// 参数:
//   - id: 会话 ID（为空则生成新 ID）
//
// 返回: Session 实例和可能的错误
func (s *Store) GetOrCreate(id string) (*Session, error) {
	if id == "" {
		id = generateSessionID()
	}

	if session, ok := s.cache[id]; ok {
		return session, nil
	}

	// 尝试从文件加载
	filePath := s.sessionFilePath(id)
	if data, err := os.ReadFile(filePath); err == nil {
		session, err := s.loadFromFile(filePath, data)
		if err == nil {
			s.cache[id] = session
			return session, nil
		}
		// 加载失败，继续创建新的
	}

	// 创建新会话
	session := &Session{
		ID:        id,
		Title:     "New Session",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		messages:  make([]*schema.Message, 0),
		filePath:  filePath,
	}
	s.cache[id] = session
	return session, nil
}

// List 返回所有会话列表
// 返回: Session 列表（按更新时间倒序）
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read session dir: %w", err)
	}

	sessions := make([]*Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		id := entry.Name()[:len(entry.Name())-6] // 去掉 .jsonl 后缀
		session, err := s.GetOrCreate(id)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}

	// 按更新时间倒序
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].UpdatedAt.After(sessions[i].UpdatedAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	return sessions, nil
}

// Delete 删除会话
// 参数:
//   - id: 会话 ID
//
// 返回: 可能的错误
func (s *Store) Delete(id string) error {
	filePath := s.sessionFilePath(id)

	// 从缓存移除
	if _, ok := s.cache[id]; ok {
		delete(s.cache, id)
	}

	// 删除文件
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session file: %w", err)
	}

	return nil
}

// GetSessionDir 返回会话存储目录
func (s *Store) GetSessionDir() string {
	return s.dir
}

// GetLatestID 返回最新会话的 ID（按更新时间倒序）
// 返回: 最新会话 ID，如果不存在则返回空字符串
func (s *Store) GetLatestID() (string, error) {
	sessions, err := s.List()
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", nil
	}
	return sessions[0].ID, nil
}

// Append 追加消息到会话并持久化
// 参数:
//   - session: Session 实例
//   - msg: 要添加的消息
//
// 返回: 可能的错误
func (s *Store) Append(session *Session, msg *schema.Message) error {
	session.messages = append(session.messages, msg)
	session.UpdatedAt = time.Now().UTC()
	session.dirty = true

	// 如果是第一条用户消息，生成标题
	if session.Title == "New Session" && msg.Role == schema.User {
		session.Title = generateTitle(msg.Content)
	}

	return s.saveToFile(session)
}

// ReplaceMessages replaces all session messages and persists the session.
func (s *Store) ReplaceMessages(session *Session, messages []*schema.Message) error {
	session.messages = messages
	session.UpdatedAt = time.Now().UTC()
	session.dirty = true
	return s.saveToFile(session)
}

// GetMessages 返回会话的所有消息
// 参数:
//   - session: Session 实例
//
// 返回: 消息列表
func (s *Store) GetMessages(session *Session) []*schema.Message {
	return session.messages
}

// LoadMessages 加载会话的所有消息（从文件）
// 参数:
//   - session: Session 实例
//
// 返回: 消息列表和可能的错误
func (s *Store) LoadMessages(session *Session) ([]*schema.Message, error) {
	if len(session.messages) > 0 {
		return session.messages, nil
	}

	data, err := os.ReadFile(session.filePath)
	if err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}

	return s.parseMessages(data)
}

func (s *Store) sessionFilePath(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

func (s *Store) saveToFile(session *Session) error {
	if !session.dirty {
		return nil
	}

	var entries []sessionFileEntry

	// 添加会话头
	entries = append(entries, sessionFileEntry{
		Type:      "session",
		ID:        session.ID,
		Title:     session.Title,
		CreatedAt: session.CreatedAt.Format(time.RFC3339),
		UpdatedAt: session.UpdatedAt.Format(time.RFC3339),
	})

	// 添加消息
	for _, msg := range session.messages {
		entry := sessionFileEntry{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
		}
		entries = append(entries, entry)
	}

	// 写入文件
	file, err := os.OpenFile(session.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	defer file.Close()

	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		if _, err := file.Write(append(line, '\n')); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}

	session.dirty = false
	return nil
}

func (s *Store) loadFromFile(filePath string, data []byte) (*Session, error) {
	session := &Session{filePath: filePath}
	for _, line := range splitJSONLines(data) {
		var entry sessionFileEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type == "session" {
			session.ID, session.Title = entry.ID, entry.Title
			if t, err := time.Parse(time.RFC3339, entry.CreatedAt); err == nil {
				session.CreatedAt = t
			}
			if t, err := time.Parse(time.RFC3339, entry.UpdatedAt); err == nil {
				session.UpdatedAt = t
			}
		} else if entry.Role != "" {
			session.messages = append(session.messages, messageFromEntry(entry))
		}
	}
	return session, nil
}

func (s *Store) parseMessages(data []byte) ([]*schema.Message, error) {
	var messages []*schema.Message
	for _, line := range splitJSONLines(data) {
		var entry sessionFileEntry
		if err := json.Unmarshal(line, &entry); err != nil || entry.Role == "" || entry.Type == "session" {
			continue
		}
		messages = append(messages, messageFromEntry(entry))
	}
	return messages, nil
}

func messageFromEntry(entry sessionFileEntry) *schema.Message {
	return &schema.Message{
		Role:       parseRole(entry.Role),
		Content:    entry.Content,
		ToolCalls:  entry.ToolCalls,
		ToolCallID: entry.ToolCallID,
		ToolName:   entry.ToolName,
	}
}

// parseRole 将字符串 role 映射为 schema.RoleType
func parseRole(role string) schema.RoleType {
	switch strings.ToLower(role) {
	case "user":
		return schema.User
	case "assistant":
		return schema.Assistant
	case "system":
		return schema.System
	case "tool":
		return schema.Tool
	default:
		return schema.User
	}
}

func splitJSONLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if line := data[start:i]; len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if line := data[start:]; len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

func generateSessionID() string {
	return fmt.Sprintf("%s", time.Now().UTC().Format("20060102150405"))
}

func generateTitle(content string) string {
	if len(content) <= 30 {
		return content
	}
	return content[:27] + "..."
}
