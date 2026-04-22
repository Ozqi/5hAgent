# Skill 注入机制

```text
.5hagent/skills/*/SKILL.md
  -> internal/skill/skill.go LoadSkills()
  -> internal/agent/agent.go injectSkills()
  -> 首次对话时作为独立 system message 注入
```

## 概述

当前实现把 skill 视为可选的 system prompt 片段。

- skill 定义存放在 `.5hagent/skills/*/SKILL.md`
- skill 默认加载但不启用
- 启用后会在首次对话时注入上下文
- 注入形式是独立 `system` 消息，不与主 `SystemPrompt` 混写

## 关键文件

- `internal/skill/skill.go`: skill 定义与加载器
- `internal/agent/agent.go`: `injectSkills()`
- `internal/tools/skill_tool.go`: LLM 调用 skill 的工具接口
- `internal/commands/skill.go`: CLI `/skill` 命令

## 目录结构

```text
.5hagent/
└── skills/
    ├── using-superpowers/
    │   └── SKILL.md
    ├── brainstorming/
    │   └── SKILL.md
    ├── writing-plans/
    │   └── SKILL.md
    └── ...
```

仓库当前实际 skill 集合以 `.5hagent/skills/` 目录为准，不应再写死旧的 `code-review` / `debug-helper` / `superpower` 示例为“内置技能”。

## Skill 文件格式

当前 loader 依赖 Markdown + YAML frontmatter：

```markdown
---
name: my-skill
description: Use when user asks to "..."
version: 1.0.0
tools: Read, Grep
---

# My Skill

Skill content...
```

已解析字段：

- `name`
- `description`
- `version`
- `tools`

运行时字段：

- `Content`
- `Enabled`

## 加载流程

`skill.Manager.LoadSkills()` 的逻辑是：

1. 读取 `.5hagent/skills/` 下所有子目录
2. 查找每个子目录中的 `SKILL.md`
3. 解析 frontmatter 和正文
4. 将结果缓存到 `map[string]*Skill`
5. 默认设置为禁用

## 注入流程

`Agent.Run()` 和 `Agent.RunStream()` 在首次对话时执行：

1. 注入主 system prompt
2. 调用 `injectSkills()`
3. 将所有 `Enabled == true` 的 skill 追加为独立 system message
4. 再追加用户消息

消息顺序大致如下：

```text
system: main agent prompt
system: # Skill: using-superpowers
system: # Skill: writing-plans
user:   ...
```

## 启用方式

### CLI 命令

- `/skill list`
- `/skill enable <name>`
- `/skill disable <name>`

这部分不经过 LLM，直接调用 `skill.Manager`。

### LLM 工具

`skill` tool 允许模型自己启用或禁用 skill：

- `{"skill":"using-superpowers","action":"enable"}`
- `{"skill":"writing-plans","action":"disable"}`

## 当前实现边界

- skill 状态是运行时内存状态，没有独立持久化文件。
- skill 只在首次对话时注入；已经开始的上下文不会自动重放新启用的 skill。
- loader 只读取 `SKILL.md`，不会解析 skill 目录下的其它辅助文件。

## 添加新 Skill

1. 创建目录 `.5hagent/skills/<name>/`
2. 新建 `SKILL.md`
3. 填写 frontmatter 和正文
4. 重启程序后重新加载

## 相关代码

- [skill.go](../internal/skill/skill.go)
- [agent.go](../internal/agent/agent.go)
- [skill_tool.go](../internal/tools/skill_tool.go)
- [skill command](../internal/commands/skill.go)
