package context

import (
	"github.com/cloudwego/eino/schema"
)

// Context Agent 运行上下文
// 每个 Agent 实例拥有独立的 Context
type Context struct {
	messages []*schema.Message
}

// Manager 上下文管理器
// 负责创建、克隆、管理多个 Context 实例
type Manager struct {
	// 最简单的实现，不需要额外字段
}

// NewManager 创建新的上下文管理器
// 返回: Manager 实例
func NewManager() *Manager {
	return &Manager{}
}

// CreateContext 创建新的 Context
// 返回: Context 实例和可能的错误
// 功能: 创建一个空的 Context
func (m *Manager) CreateContext() (*Context, error) {
	return &Context{
		messages: make([]*schema.Message, 0),
	}, nil
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
