# 5hAgent 文档索引

## 核心模块

### [Agent - 核心模块](agent.md)
Agent 的核心实现，包括 ReAct 循环、工具执行、Skill 注入、TaskList 管理。

**关键内容**:
- Agent 结构和配置
- Run/RunStream 流式输出
- 工具并发执行策略
- Skill 注入机制
- TaskList 任务管理

**文件**: `internal/agent/agent.go` (~822行)

---

### [Tools - 工具系统](tools.md)
工具系统的实现，包括文件操作、搜索、执行、任务管理等 12 个工具。

**关键内容**:
- 12 个内置工具（6 只读 + 6 写入）
- 工具注册机制
- 并发执行策略
- 添加新工具的方法

**文件**: `internal/tools/*.go`

---

### [Context - 上下文管理](context.md)
上下文管理器，负责消息存储、压缩和管理。

**关键内容**:
- Context 结构
- 消息添加和获取
- 自动压缩机制（50 → 30 条）
- 压缩策略和未来优化

**文件**: `internal/context/ctx.go` (~113行)

---

### [Skill - 技能注入系统](skill_injection.md)
Skill 注入机制，支持动态加载和启用预定义技能。

**关键内容**:
- Skill 文件格式（Markdown + YAML frontmatter）
- Skill 管理器
- 注入时机和方式
- 内置技能（code-review, debug-helper, superpower）
- 创建新技能的方法

**文件**: `internal/skill/skill.go` (~151行)

---

### [Prompt - 提示词管理](prompt.md)
提示词管理系统，负责加载、解析和管理各类提示词。

**关键内容**:
- Prompt 结构和加载器
- 提示词文件格式
- 变量替换机制
- 内置提示词（main_agent, task_planner, code_reviewer 等）

**文件**: `internal/prompt/loader.go` (~182行)

---

### [Commands - 命令处理](commonds.md)
处理用户的斜杠命令（/skill, /task），不涉及 LLM 调用。

**关键内容**:
- /skill 命令（list, enable, disable）
- /task 命令（list, create, update, get, delete）
- 命令处理流程
- 扩展新命令的方法

**文件**: `internal/commands/*.go`

---

## 辅助模块

### [LLM - 大模型客户端](llm.md)
大模型客户端封装，基于 Eino 框架的 Claude 模型。

**关键内容**:
- Config 配置结构
- 从环境变量创建客户端
- .env 文件配置
- 与 Agent 的集成

**文件**: `internal/llm/client.go` (~107行)

---

### [Logger - 日志系统](logger.md)
日志模块，提供 4 级日志、标签分类、彩色输出。

**关键内容**:
- 日志级别（DEBUG/INFO/WARN/ERROR）
- 标签系统
- 颜色方案
- 格式对齐

**文件**: `internal/logger/logger.go`, `internal/logger/color.go`

---

### [CLI - 命令行界面](cli.md)
命令行界面工具，用于美化用户交互体验。

**关键内容**:
- PrintUserInput
- PrintAssistantChunk（流式输出）
- PrintToolCall/PrintToolResult
- PrintError

**文件**: `internal/cli/ui.go` (~37行)

---

## 其他文档

### [SWE-bench 接入计划](swe_bench_integration.md)
评估 5hAgent 在真实软件工程任务上的表现。

**关键内容**:
- 当前能力评估
- 接入方案（4 个阶段）
- 快速开始方法
- 预期结果

---

## 文档结构

```
doc/
├── README.md                    # 本文档（索引）
├── agent.md                     # Agent 核心模块
├── tools.md                     # 工具系统
├── context.md                   # 上下文管理
├── skill_injection.md           # Skill 注入系统
├── prompt.md                    # 提示词管理
├── commonds.md                  # 命令处理
├── llm.md                       # LLM 客户端
├── logger.md                    # 日志系统
├── cli.md                       # CLI 工具
└── swe_bench_integration.md     # SWE-bench 接入
```

## 代码结构

```
5hAgent/
├── cmd/5hagent/                 # 主程序入口
├── internal/
│   ├── agent/                   # Agent 核心
│   ├── tools/                   # 工具实现
│   ├── context/                 # 上下文管理
│   ├── skill/                   # Skill 管理
│   ├── prompt/                  # 提示词加载
│   ├── commands/                # 命令处理
│   ├── llm/                     # LLM 客户端
│   ├── logger/                  # 日志系统
│   ├── cli/                     # CLI 工具
│   └── utils/                   # 工具函数
├── prompt/system/               # 系统提示词
├── .5hagent/
│   ├── skills/                  # 技能定义
│   └── tasks.json               # 任务持久化
└── doc/                         # 文档目录
```

## 快速导航

### 我想了解...

- **如何添加新工具？** → [tools.md - 添加新工具](tools.md#添加新工具)
- **如何创建新技能？** → [skill_injection.md - 创建新技能](skill_injection.md#创建新技能)
- **如何管理提示词？** → [prompt.md - 提示词文件格式](prompt.md#提示词文件格式)
- **工具如何并发执行？** → [agent.md - 工具并发执行](agent.md#exetools-多路执行)
- **上下文如何压缩？** → [context.md - 压缩策略](context.md#压缩策略)
- **如何添加新命令？** → [commonds.md - 扩展新命令](commonds.md#扩展新命令)

### 我想修改...

- **Agent 行为** → [agent.md](agent.md) + [prompt.md](prompt.md)
- **工具功能** → [tools.md](tools.md)
- **日志输出** → [logger.md](logger.md)
- **用户界面** → [cli.md](cli.md)
- **LLM 配置** → [llm.md](llm.md)

## 文档规范

### 代码引用链接格式

使用相对路径 + 行号范围：

```markdown
[agent.go:478-605](../internal/agent/agent.go#L478-L605)
```

### 文档结构

1. **位置** - 文件路径和行数
2. **概述** - 模块功能简介
3. **核心组件/函数** - 详细说明
4. **使用示例** - 代码示例
5. **设计特点** - 设计思路
6. **未来扩展** - 待实现功能
7. **相关文件** - 关联文件列表

### 更新文档

代码改动后，同步更新文档：
1. 检查行号引用是否正确
2. 更新函数签名和结构体定义
3. 补充新增功能的说明
4. 更新代码示例

---

**最后更新**: 2026-04-22
