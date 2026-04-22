# Commands - 命令处理模块

## 位置

`internal/commands/` 目录

## 概述

处理用户的斜杠命令，完全不涉及 LLM 调用，直接操作内部状态。

## 命令列表

### /skill - 技能管理

**文件**: `internal/commands/skill.go`

**功能**: 管理 Agent 的技能启用/禁用状态

**子命令**:
- `/skill list` - 列出所有可用技能及其状态
- `/skill enable <name>` - 启用指定技能
- `/skill disable <name>` - 禁用指定技能

**实现**: 调用 `skill.Manager` 的方法

**代码链接**: [skill.go](../internal/commands/skill.go)

### /task - 任务管理

**文件**: `internal/commands/task.go`

**功能**: 管理持久化任务列表

**子命令**:
- `/task list [status]` - 列出所有任务（可选按状态过滤）
- `/task create <id> <title> <description>` - 创建新任务
- `/task update <id> <status>` - 更新任务状态
- `/task get <id>` - 获取任务详情
- `/task delete <id>` - 删除任务

**实现**: 调用 `agent.TaskList` 的方法

**代码链接**: [task.go](../internal/commands/task.go)

## 命令处理流程

1. 用户在 CLI 输入斜杠命令（如 `/skill list`）
2. `cmd/5hagent/main.go` 检测到命令前缀
3. 根据命令类型调用对应的 `commands.HandleXxx()` 函数
4. 命令处理函数解析参数并调用底层模块
5. 返回结果字符串显示给用户

## 设计原则

- **无 LLM 调用**: 命令处理不涉及大模型，响应速度快
- **直接操作**: 直接修改内部状态（技能启用、任务列表）
- **简单解析**: 使用 `strings.Fields()` 分割参数
- **错误友好**: 参数错误时返回 usage 提示

## 扩展新命令

1. 在 `internal/commands/` 创建新文件（如 `memory.go`）
2. 实现 `HandleXxx(cmd string, deps...) (string, error)` 函数
3. 在 `cmd/5hagent/main.go` 的命令分发逻辑中添加分支
4. 更新本文档

## 相关文件

- `cmd/5hagent/main.go` - 命令分发入口
- `internal/skill/skill.go` - 技能管理器
- `internal/agent/tasklist.go` - 任务列表
