# CLI - 命令行界面工具

## 位置

`internal/cli/ui.go` (~37行)

## 概述

CLI 模块提供命令行界面的输出格式化工具，用于美化用户交互体验。

## 核心函数

### PrintUserInput(text)

打印用户输入，带 "User:" 前缀。

**参数**: `text` - 用户输入文本

**输出示例**:
```
User: 读取 task.md 文件
```

**代码链接**: [ui.go:9-11](../internal/cli/ui.go#L9-L11)

### PrintAssistantChunk(text)

打印 AI 响应片段（流式输出，不换行）。

**参数**: `text` - 响应文本片段

**用途**: 流式输出时逐字显示 AI 响应

**代码链接**: [ui.go:14-16](../internal/cli/ui.go#L14-L16)

### PrintToolCall(name, input)

打印工具调用信息。

**参数**:
- `name` - 工具名称
- `input` - 工具输入参数

**输出示例**:
```
[Tool] base.read_file({"path": "task.md"})
```

**代码链接**: [ui.go:19-21](../internal/cli/ui.go#L19-L21)

### PrintToolResult(result)

打印工具执行结果，带缩进。

**参数**: `result` - 工具返回结果

**输出示例**:
```
  Result: {"content": "...", "total_lines": 58}
```

**代码链接**: [ui.go:24-31](../internal/cli/ui.go#L24-L31)

### PrintError(err)

打印错误消息，使用红色 ANSI 颜色。

**参数**: `err` - 错误对象

**输出示例**:
```
Error: failed to read file
```

**代码链接**: [ui.go:34-36](../internal/cli/ui.go#L34-L36)

## 使用示例

```go
// 打印用户输入
cli.PrintUserInput("读取 task.md")

// 流式输出 AI 响应
cli.PrintAssistantChunk("正在")
cli.PrintAssistantChunk("读取")
cli.PrintAssistantChunk("文件...")

// 打印工具调用
cli.PrintToolCall("base.read_file", `{"path": "task.md"}`)

// 打印工具结果
cli.PrintToolResult(`{"content": "...", "total_lines": 58}`)

// 打印错误
cli.PrintError(fmt.Errorf("file not found"))
```

## 设计特点

### 1. 简洁接口

- 函数命名清晰
- 参数简单直观
- 无复杂配置

### 2. 流式友好

- `PrintAssistantChunk` 不换行
- 支持逐字输出效果
- 提升用户体验

### 3. 视觉区分

- 用户输入有前缀
- 工具调用用 `[Tool]` 标记
- 工具结果带缩进
- 错误消息用红色

## 与 Logger 的区别

| 特性 | CLI 模块 | Logger 模块 |
|------|---------|-----------|
| 用途 | 用户界面输出 | 调试和日志记录 |
| 目标受众 | 最终用户 | 开发者 |
| 输出格式 | 简洁友好 | 详细结构化 |
| 颜色支持 | 基础（仅错误） | 丰富（级别、标签） |
| 日志级别 | 无 | DEBUG/INFO/WARN/ERROR |
| 时间戳 | 无 | 有 |

## 未来扩展

- [ ] 支持更丰富的颜色方案
- [ ] 支持进度条和加载动画
- [ ] 支持表格和列表格式化
- [ ] 支持 Markdown 渲染
- [ ] 支持交互式提示（选择、确认）
- [ ] 支持主题配置

## 相关文件

- `internal/cli/ui.go` - UI 工具实现
- `internal/logger/logger.go` - 日志模块（开发者视角）
- `cmd/5hagent/main.go` - 使用示例
