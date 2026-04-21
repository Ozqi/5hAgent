# Skill 注入机制实现文档

## 概述

Skill 注入机制允许动态加载和启用预定义的技能提示词，增强 Agent 在特定场景下的能力。

## 架构

```
.miniagent/skills/          # 技能定义目录
├── debug_helper.json       # 调试助手技能
└── code_review.json        # 代码审查技能

internal/skill/
└── skill.go                # 技能管理器

internal/agent/
├── agent.go                # Agent 集成技能注入
└── skill_commands.go       # /skill 命令处理
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
- `InjectSkills(basePrompt)`: 将启用的技能注入到 system prompt
- `EnableSkill(name)`: 启用指定技能
- `DisableSkill(name)`: 禁用指定技能
- `ListSkills()`: 列出所有技能

### 3. Agent 集成

在 `NewAgent()` 中初始化技能管理器，在 `Run()` 和 `RunStream()` 中注入技能到 system prompt。

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
