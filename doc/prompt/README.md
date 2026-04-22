# Prompt 管理系统

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      Application Layer                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  main.go     │  │  agent.go    │  │  skill.go    │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                 │                  │               │
│         └─────────────────┼──────────────────┘               │
│                           │                                  │
│                           ▼                                  │
│  ┌────────────────────────────────────────────────────────┐ │
│  │           Prompt Loader (loader.go)                    │ │
│  │  - Load()              加载所有提示词                   │ │
│  │  - Get(name)           获取提示词对象                   │ │
│  │  - GetContent(name)    获取内容                        │ │
│  │  - GetContentWithVars  变量替换                        │ │
│  └────────────────────────┬───────────────────────────────┘ │
└───────────────────────────┼─────────────────────────────────┘
                            │
                            ▼
        ┌───────────────────────────────────────┐
        │      Prompt Files (*.md)              │
        │  ┌─────────────────────────────────┐  │
        │  │  YAML Frontmatter               │  │
        │  │  - name: 唯一标识               │  │
        │  │  - type: system/user/assistant  │  │
        │  │  - variables: 变量定义          │  │
        │  └─────────────────────────────────┘  │
        │  ┌─────────────────────────────────┐  │
        │  │  Markdown Content               │  │
        │  │  - 支持 {{variable}} 语法      │  │
        │  └─────────────────────────────────┘  │
        └───────────────────────────────────────┘
```

## 核心实现

### 1. Loader 结构 ([loader.go](../../internal/prompt/loader.go))

```go
type Loader struct {
    promptsDir string
    prompts    map[string]*Prompt
}
```

关键函数：
- [`Load()`](../../internal/prompt/loader.go#L47) - 遍历目录加载所有 .md 文件
- [`loadPromptFile()`](../../internal/prompt/loader.go#L88) - 解析单个文件的 frontmatter 和内容
- [`GetContentWithVars()`](../../internal/prompt/loader.go#L151) - 替换 `{{var}}` 变量

### 2. Prompt 结构 ([loader.go](../../internal/prompt/loader.go#L13))

```go
type Prompt struct {
    Name        string            // 唯一标识
    Type        string            // system/user/assistant
    Variables   map[string]string // 变量定义
    Content     string            // 实际内容
}
```

### 3. 文件格式

```markdown
---
name: prompt_name
type: system
variables:
  var1: "default"
---
内容 {{var1}}
```

## 使用方式

### 基本用法 ([main.go](../../cmd/5hagent/main.go#L72))

```go
loader := prompt.NewLoader("prompt")
loader.Load()
systemPrompt, _ := loader.GetContent("main_agent_system")
```

### 变量替换

```go
vars := map[string]string{"task_description": "具体任务"}
content, _ := loader.GetContentWithVars("sub_agent_template", vars)
```

## 已有提示词

| 文件 | name | 用途 | 变量 |
|------|------|------|------|
| [main_agent.md](../../prompt/system/main_agent.md) | main_agent_system | 主 Agent 系统提示词 | - |
| [sub_agent_template.md](../../prompt/system/sub_agent_template.md) | sub_agent_template | Sub-Agent 模板 | task_description, constraints |
| [task_planner.md](../../prompt/system/task_planner.md) | task_planner | 任务规划 | - |
| [code_reviewer.md](../../prompt/system/code_reviewer.md) | code_reviewer | 代码审查 | - |
| [debug_helper.md](../../prompt/system/debug_helper.md) | debug_helper | Debug 辅助 | - |
| [context_compressor.md](../../prompt/system/context_compressor.md) | context_compressor | 上下文压缩 | max_tokens |

## 测试

```bash
go test ./internal/prompt/...  # 单元测试
```

测试覆盖：基本加载、变量替换、错误处理、无效格式
