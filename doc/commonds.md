# Commands - 斜杠命令

```text
REPL input
  -> cmd/5hagent/main.go
     -> /skill => internal/commands/skill.go
     -> /task  => internal/commands/task.go
     -> /compress => internal/commands/compress.go
```

## 概述

这里记录的是 CLI 斜杠命令，不是 LLM tool。

- 斜杠命令由主 REPL 直接解析
- 不经过 LLM
- 返回值直接打印到终端

## 当前命令

### `/skill`

文件：`internal/commands/skill.go`

支持：

- `/skill list`
- `/skill enable <name>`
- `/skill disable <name>`

底层依赖：`internal/skill/skill.go`

### `/task`

文件：`internal/commands/task.go`

支持：

- `/task list [status]`
- `/task create <id> <title> <description>`
- `/task update <id> <status>`
- `/task get <id>`
- `/task delete <id>`

底层依赖：`internal/task/tasklist.go`

## 处理流程

1. REPL 读到一行输入
2. `main.go` 判断是否以 `/skill`、`/task` 或 `/compress` 开头
3. 调用对应的 `HandleSkill()`、`HandleTask()` 或 `HandleCompress()`
4. 函数内部用 `strings.Fields()` 解析参数
5. 返回结果字符串或错误

## 与 LLM 工具的关系

当前仓库同时存在：

- CLI 命令：`/skill`、`/task`、`/compress`
- LLM 工具：`skill.skill`、`task.task`

两者职责类似，但入口不同：

- CLI 命令给用户直接操作
- LLM 工具给 Agent 自主调用
- `/compress` 走手动上下文压缩，并把归档写到 `compact/messages/`，同时打印压缩后的当前 ctx

## 扩展方式

1. 在 `internal/commands/` 新增命令处理文件
2. 实现 `HandleXxx(...)`
3. 在 `cmd/5hagent/main.go` 中添加分支
4. 同步更新本文档

## 相关代码

- [main.go](../cmd/5hagent/main.go)
- [skill.go](../internal/commands/skill.go)
- [task.go](../internal/commands/task.go)
- [compress.go](../internal/commands/compress.go)
