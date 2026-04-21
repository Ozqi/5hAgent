# Skill 注入机制实现文档

## 概述

Skill 注入机制允许动态加载和启用预定义的技能提示词，增强 Agent 在特定场景下的能力。技能作为独立的 System 消息插入到对话上下文中，而不是混入主 System Prompt。

## 架构

```
.miniagent/skills/          # 技能定义目录
├── debug_helper.json       # 调试助手技能
└── code_review.json        # 代码审查技能

internal/skill/
└── skill.go                # 技能管理器

internal/agent/
├── agent.go                # Agent 集成技能注入
└── skill.go                # 技能注入和命令处理
```

## 核心组件

### 1. Skill 结构

```go
type Skill struct {
    Name        string `json:"name"`        // 技能名称
    Description string `json:"description"` // 技能描述
    Prompt      string `json:"prompt"`      // 技能提示词
    Enabled     bool   `json:"enabled"`     // 是否启用
}
```

### 2. Manager 技能管理器

**功能**:
- `LoadSkills()`: 从 `.miniagent/skills/*.json` 加载技能定义
- `EnableSkill(name)`: 启用指定技能
- `DisableSkill(name)`: 禁用指定技能
- `ListSkills()`: 列出所有技能

### 3. Agent 集成

**注入时机**: 在首次对话时，System Prompt 之后、User Message 之前

**注入方式**: 每个启用的技能作为独立的 System 消息插入

```go
// 消息顺序示例
[
  {role: "system", content: "You are a helpful AI assistant."},
  {role: "system", content: "# Skill: debug_helper\n..."},
  {role: "system", content: "# Skill: code_review\n..."},
  {role: "user", content: "用户输入"}
]
```

**优势**:
- 职责分离：System Prompt 和 Skill 独立管理
- 动态性：可以在不修改 System Prompt 的情况下启用/禁用技能
- 可追踪：每个技能作为独立消息，便于调试和日志记录

## 使用方式

### 命令行交互

```bash
# 列出所有技能
/skill list

# 启用技能
/skill enable debug_helper

# 禁用技能
/skill disable debug_helper
```

### 技能定义格式

```json
{
  "name": "skill_name",
  "description": "技能描述",
  "prompt": "详细的提示词内容",
  "enabled": false
}
```

## 内置技能

### debug_helper
调试助手，提供系统化的调试方法论。

### code_review
代码审查技能，关注代码质量、安全性、性能等方面。

## 扩展

添加新技能只需在 `.miniagent/skills/` 目录下创建新的 JSON 文件，Agent 启动时会自动加载。

## 实现细节

### injectSkills() 函数

位于 `internal/agent/skill.go`，负责将启用的技能转换为 System 消息并插入到上下文中。

```go
func (a *Agent) injectSkills(messageCtx *agentctx.Context) error {
    skills := a.skillManager.ListSkills()
    for _, skill := range skills {
        if skill.Enabled {
            skillMsg := &schema.Message{
                Role:    schema.System,
                Content: fmt.Sprintf("# Skill: %s\n%s\n\n%s", 
                    skill.Name, skill.Description, skill.Prompt),
            }
            if err := a.ctxManager.AddMessage(messageCtx, skillMsg); err != nil {
                return fmt.Errorf("failed to add skill %s: %w", skill.Name, err)
            }
        }
    }
    return nil
}
```

### 注入流程

1. 检查是否为首次对话（消息列表为空）
2. 添加主 System Prompt
3. 调用 `injectSkills()` 添加启用的技能
4. 添加用户消息
5. 开始 ReAct 循环
