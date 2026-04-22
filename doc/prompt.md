# Prompt - 提示词管理

```text
prompt/**/*.md
  -> internal/prompt/loader.go Load()
  -> 以 frontmatter.name 为 key 建表
  -> cmd/5hagent/main.go 读取 main_agent_system
```

## 位置

- `internal/prompt/loader.go`
- `internal/prompt/loader_test.go`
- `prompt/**/*.md`

## 概述

Prompt loader 负责从 `prompt/` 目录递归读取 Markdown 文件，解析 YAML frontmatter，并把正文作为 prompt 内容缓存起来。

当前主流程在 `cmd/5hagent/main.go` 中：

1. `prompt.NewLoader("prompt")`
2. `Load()`
3. `GetContent("main_agent_system")`
4. 将结果写入 `agent.Config.SystemPrompt`

## Prompt 结构

`internal/prompt/loader.go` 中的 `Prompt` 字段：

- `Name`
- `Description`
- `Version`
- `Type`
- `Variables`
- `Content`

## 文件格式

```markdown
---
name: main_agent_system
description: 主 Agent 的系统提示词
version: "1.0"
type: system
---

Prompt body...
```

loader 要求：

- 文件必须以 `---\n` 开头
- frontmatter 与正文之间必须由 `\n---\n` 分隔
- 文件后缀必须是 `.md`

## Loader 接口

- `NewLoader(promptsDir)`
- `Load()`
- `Get(name)`
- `GetContent(name)`
- `GetContentWithVars(name, vars)`
- `List()`
- `Reload()`

## 当前已存在的 Prompt 文件

当前仓库中的 prompt 文件分布在：

- `prompt/system/*.md`
- `prompt/*.md`

其中真正用于 CLI 主流程的是：

- `prompt/system/main_agent.md`

其 frontmatter `name` 为 `main_agent_system`。

## 变量替换

`GetContentWithVars()` 只做简单字符串替换：

- 占位符格式：`{{name}}`
- 实现方式：`strings.ReplaceAll`

它不做模板语法校验，也不会检查未替换变量。

## 当前实现边界

- loader 会递归遍历整个 `prompt/` 目录，而不只是 `prompt/system/`。
- 无效 prompt 文件会被跳过，不会中断整个加载过程。
- 当前没有 prompt 版本选择、继承或优先级机制。

## 相关代码

- [loader.go](../internal/prompt/loader.go)
- [loader_test.go](../internal/prompt/loader_test.go)
- [main.go](../cmd/5hagent/main.go)
- [main_agent.md](../prompt/system/main_agent.md)
