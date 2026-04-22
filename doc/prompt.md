# Prompt - 提示词管理系统

## 位置

- `internal/prompt/loader.go` (~182行) - 提示词加载器
- `prompt/system/*.md` - 系统提示词文件

## 概述

提示词管理系统负责加载、解析和管理 Agent 使用的各类提示词。采用 Markdown + YAML frontmatter 格式，支持变量替换。

## 架构

```
prompt/
├── system/                      # 系统提示词目录
│   ├── main_agent.md           # 主 Agent 提示词
│   ├── sub_agent_template.md   # 子 Agent 模板
│   ├── task_planner.md         # 任务规划提示词
│   ├── code_reviewer.md        # 代码审查提示词
│   ├── debug_helper.md         # 调试助手提示词
│   └── context_compressor.md   # 上下文压缩提示词
└── README.md                    # 提示词文档

internal/prompt/
└── loader.go                    # 提示词加载器实现
```

## 核心组件

### 1. Prompt 结构

```go
type Prompt struct {
    Name        string            // 提示词名称
    Description string            // 提示词描述
    Version     string            // 版本号
    Type        string            // 类型: system/user/assistant
    Variables   map[string]string // 变量定义（可选）
    Content     string            // 提示词内容（不在 frontmatter 中）
}
```

### 2. Loader 加载器

**功能**:
- `NewLoader(promptsDir)`: 创建加载器实例
- `Load()`: 加载目录下所有 .md 文件
- `Get(name)`: 获取指定提示词
- `GetContent(name)`: 获取提示词内容
- `GetContentWithVars(name, vars)`: 获取内容并替换变量
- `List()`: 列出所有提示词
- `Reload()`: 重新加载所有提示词

**代码链接**: [loader.go:33-181](../internal/prompt/loader.go#L33-L181)

## 提示词文件格式

### 标准结构

```markdown
---
name: prompt-name
description: Brief description of this prompt
version: 1.0.0
type: system
variables:
  task_description: "Task to be completed"
  context: "Additional context"
---

# Prompt Title

Prompt content in Markdown format...

You are {{role}}. Your task is to {{task_description}}.

## Instructions

1. Step one
2. Step two

## Context

{{context}}
```

### Frontmatter 字段

- **name** (必需): 提示词标识符，用于 `loader.Get(name)`
- **description** (必需): 提示词用途描述
- **version** (可选): 语义化版本号
- **type** (可选): 提示词类型（system/user/assistant）
- **variables** (可选): 变量定义及默认值

### 变量替换

使用 `{{variable}}` 格式定义变量占位符：

```markdown
You are a {{role}} assistant. Your task is to {{task}}.
```

调用时传入变量映射：

```go
content, err := loader.GetContentWithVars("main_agent", map[string]string{
    "role": "helpful",
    "task": "answer user questions",
})
```

## 使用示例

### 初始化加载器

```go
// cmd/5hagent/main.go
loader := prompt.NewLoader("prompt/system")
if err := loader.Load(); err != nil {
    log.Fatal(err)
}
```

### 获取提示词

```go
// 获取原始内容
content, err := loader.GetContent("main_agent")

// 获取并替换变量
content, err := loader.GetContentWithVars("task_planner", map[string]string{
    "task_description": "实现用户登录功能",
    "context": "使用 JWT 认证",
})
```

### 列出所有提示词

```go
prompts := loader.List()
for _, p := range prompts {
    fmt.Printf("%s: %s (v%s)\n", p.Name, p.Description, p.Version)
}
```

## 内置提示词

### main_agent.md

主 Agent 的系统提示词，定义 Agent 的角色、能力和行为规范。

**变量**: 无

**用途**: Agent 初始化时注入

### sub_agent_template.md

子 Agent 的提示词模板，用于创建专门执行特定任务的子 Agent。

**变量**: `task_description`, `context`

**用途**: 动态创建子 Agent

### task_planner.md

任务规划提示词，指导 Agent 如何分解和规划复杂任务。

**变量**: `task_description`

**用途**: 长程任务规划

### code_reviewer.md

代码审查提示词，提供代码质量、安全性、性能审查清单。

**变量**: 无

**用途**: 代码审查场景

### debug_helper.md

调试助手提示词，提供系统化的调试方法论。

**变量**: 无

**用途**: 调试场景

### context_compressor.md

上下文压缩提示词，指导如何总结和压缩对话历史。

**变量**: `messages`

**用途**: 上下文压缩（未来实现）

## 加载流程

1. **初始化**: 创建 Loader 实例，指定提示词目录
2. **遍历目录**: 使用 `filepath.Walk` 遍历所有 .md 文件
3. **解析文件**: 
   - 检查 frontmatter 格式（`---\n...---\n`）
   - 使用 `yaml.Unmarshal` 解析 frontmatter
   - 提取 Markdown 内容
4. **存储**: 将 Prompt 实例存入 `map[string]*Prompt`
5. **查询**: 通过 name 快速查找

**代码链接**: [loader.go:47-77](../internal/prompt/loader.go#L47-L77)

## 设计原则

### 1. 分离关注点

- 提示词内容与代码分离
- 便于非开发人员编辑提示词
- 支持版本控制和 diff

### 2. 标准化格式

- 统一使用 Markdown + YAML frontmatter
- 与 Skill 系统格式一致
- 易于阅读和维护

### 3. 变量支持

- 支持动态内容注入
- 提高提示词复用性
- 简化提示词管理

### 4. 热加载

- 支持 `Reload()` 重新加载
- 无需重启 Agent 即可更新提示词
- 便于调试和迭代

## 与 Skill 系统的区别

| 特性 | Prompt 系统 | Skill 系统 |
|------|------------|-----------|
| 用途 | Agent 核心提示词 | 可选增强能力 |
| 加载时机 | 启动时加载 | 启动时加载 |
| 启用方式 | 代码中直接使用 | 用户命令启用/禁用 |
| 变量支持 | ✅ 支持 | ❌ 不支持 |
| 存储位置 | `prompt/system/` | `.5hagent/skills/` |
| 文件格式 | Markdown + YAML | Markdown + YAML |

## 未来扩展

- [ ] 提示词版本管理（回退到旧版本）
- [ ] 提示词性能监控（token 使用统计）
- [ ] 提示词 A/B 测试
- [ ] 提示词模板继承（base + override）
- [ ] 多语言提示词支持
- [ ] 提示词优化建议（基于 LLM 反馈）

## 相关文件

- `internal/prompt/loader.go` - 加载器实现
- `internal/prompt/loader_test.go` - 单元测试
- `prompt/system/*.md` - 系统提示词
- `prompt/README.md` - 提示词文档
- `internal/skill/skill.go` - Skill 系统（类似设计）
