# Logger 日志模块

## 概述

极简日志模块，用于debug和运行时信息输出。

## 位置

`internal/logger/logger.go`

## 功能

- 4个日志级别：DEBUG, INFO, WARN, ERROR
- 线程安全（使用 sync.Mutex）
- 统一格式：`[时间] [级别] 消息`
- 可配置输出目标（默认 stdout）

## 使用

```go
import "github.com/lzq/miniAgent/internal/logger"

// 设置日志级别（默认 INFO）
logger.SetLevel(logger.DEBUG)

// 输出日志
logger.Debug("Debug message: %s", value)
logger.Info("Info message")
logger.Warn("Warning: %v", err)
logger.Error("Error occurred: %v", err)
```

## 集成点

- `cmd/miniagent/main.go:37-40` - 根据 --debug 标志设置日志级别
- `internal/agent/agent.go:134-147` - ReAct 循环关键步骤日志
- `internal/agent/agent.go:238-280` - 流式输出调试日志
- `internal/agent/agent.go:337-384` - 工具执行日志

## 设计原则

- 极简：无第三方依赖，核心代码 <100 行
- 性能：级别过滤在输出前，避免无效格式化
- 清晰：时间戳精确到秒，便于快速定位问题
