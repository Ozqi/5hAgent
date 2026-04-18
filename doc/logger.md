# Logger 日志模块

## 概述

极简日志模块，用于debug和运行时信息输出。支持标签分类和彩色输出，便于快速定位问题。

## 位置

- `internal/logger/logger.go` - 核心日志功能
- `internal/logger/color.go` - 颜色支持

## 功能

- 4个日志级别：DEBUG, INFO, WARN, ERROR
- 线程安全（使用 sync.Mutex）
- 统一格式：`[时间][级别][标签] 消息` 或 `[时间][级别] 消息`
- 彩色输出（自动检测终端支持）
- 可配置输出目标（默认 stdout）

## 颜色方案

### 日志级别
- DEBUG - 蓝色
- INFO - 绿色
- WARN - 黄色
- ERROR - 红色加粗

### 标签
- SYS - 紫色（系统）
- REACT - 青色（循环）
- LLM - 紫色（模型）
- STREAM - 蓝色（流式）
- TOOL - 青色加粗（工具）
- CTX - 灰色（上下文）
- USER - 绿色加粗（用户）
- AGENT - 黄色（代理）

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

// 颜色工具函数
fmt.Println(logger.Green("Success!"))
fmt.Println(logger.Red("Error!"))
fmt.Println(logger.Bold(logger.Cyan("Important")))
```

## 禁用颜色

设置环境变量：
```bash
export NO_COLOR=1
# 或
export TERM=dumb
```

## 集成点

- `cmd/miniagent/main.go` - 系统初始化、用户交互界面
- `internal/agent/agent.go` - ReAct循环、LLM调用、工具执行

## 设计原则

- 极简：无第三方依赖，核心代码 <200 行
- 性能：级别过滤在输出前，避免无效格式化
- 清晰：标签紧凑（3-6字符），时间戳精确到秒，颜色区分关键信息
