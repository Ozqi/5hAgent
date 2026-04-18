# Logger - 日志模块

## 位置
- `internal/logger/logger.go` - 核心日志
- `internal/logger/color.go` - 颜色支持

## 功能
4级日志(DEBUG/INFO/WARN/ERROR)，标签分类，彩色输出，线程安全

## 使用
```go
logger.SetLevel(logger.DEBUG)
logger.InfoTag("SYS", "System ready")
logger.DebugTag("TOOL", "Execute: %s", name)
fmt.Println(logger.Bold(logger.Cyan("Title")))
```

## 颜色方案
- 级别：DEBUG蓝/INFO绿/WARN黄/ERROR红粗
- 标签：SYS紫/REACT青/LLM紫/STREAM蓝/TOOL青粗/CTX灰/USER绿粗/AGENT黄

## 格式对齐
`[时间][级别][标签] 消息` - 时间/级别/标签固定宽度对齐

## 核心函数
- `logger.DebugTag(tag, format, args...)` - 带标签DEBUG日志
- `logger.TruncateString(s, maxLen)` - 截断字符串显示摘要，支持中文

## 集成点
- `cmd/miniagent/main.go:40-42` - 根据--debug设置级别
- `cmd/miniagent/main.go:68-70` - 工具注册日志
- `internal/agent/agent.go:137` - ReAct循环轮次
- `internal/agent/agent.go:144-148` - 消息上下文详情（role对齐，内容摘要）
- `internal/agent/agent.go:250` - LLM Stream调用：`reader, err := a.model.Stream(ctx, messages)`
- `internal/agent/agent.go:378-390` - 工具执行流程

## 禁用颜色
```bash
export NO_COLOR=1  # 或 TERM=dumb
```
