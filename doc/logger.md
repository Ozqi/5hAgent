# Logger 日志模块

## 概述

极简日志模块，用于debug和运行时信息输出。支持标签分类，便于快速定位问题。

## 位置

`internal/logger/logger.go`

## 功能

- 4个日志级别：DEBUG, INFO, WARN, ERROR
- 线程安全（使用 sync.Mutex）
- 统一格式：`[时间][级别][标签] 消息` 或 `[时间][级别] 消息`
- 可配置输出目标（默认 stdout）

## 使用

```go
import "github.com/lzq/miniAgent/internal/logger"

// 设置日志级别（默认 INFO）
logger.SetLevel(logger.DEBUG)

// 无标签日志
logger.Debug("Debug message: %s", value)
logger.Info("Info message")

// 带标签日志（推荐）
logger.DebugTag("TOOL", "Execute: %s", toolName)
logger.InfoTag("SYS", "System ready")
logger.WarnTag("LLM", "Retry attempt %d", n)
logger.ErrorTag("AGENT", "Failed: %v", err)
```

## 标签设计

- `SYS` - 系统初始化、配置
- `REACT` - ReAct循环控制
- `LLM` - LLM调用、响应
- `STREAM` - 流式输出细节
- `TOOL` - 工具查找、执行
- `CTX` - 上下文管理
- `USER` - 用户输入
- `AGENT` - Agent运行状态

## 集成点

- `cmd/miniagent/main.go` - 系统初始化、用户交互
- `internal/agent/agent.go` - ReAct循环、LLM调用、工具执行

## 设计原则

- 极简：无第三方依赖，核心代码 <120 行
- 性能：级别过滤在输出前，避免无效格式化
- 清晰：标签紧凑（3-6字符），时间戳精确到秒
