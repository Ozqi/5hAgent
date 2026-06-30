// ipc.go - Agent 进程间短消息协议类型
// 功能：定义 systemd、context 和 sys.ipc 工具共享的 IPC 数据结构。
// 调用方：internal/systemd 管理 mailbox，internal/context 暴露接口，internal/tools/ipc_tool 构造消息。
// 全局变量：无。
package ipctypes

import "time"

// Message 是 AgentProcess 之间的短消息。
type Message struct {
	From      string    `json:"from"`       // 发送方进程 ID
	To        string    `json:"to"`         // 接收方进程 ID
	Summary   string    `json:"summary"`    // 短消息摘要
	Artifact  string    `json:"artifact"`   // 大内容 artifact 路径，可为空
	CreatedAt time.Time `json:"created_at"` // 创建时间
}
