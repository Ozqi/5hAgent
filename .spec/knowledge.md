# Knowledge / Skill Spec

> 由 Claude Fable 5 于 2026-08-24 阅读 `internal/skill/skill.go`、`internal/tools/skill_tool.go`、`internal/commands/skill.go` 后重构。
> 覆盖范围：Skill 文件格式、加载顺序、reload 原子性和模型注入边界。

## 职责边界

`internal/skill` 只管理可注入模型的 Skill prompt 快照。Skill 不是任务系统，也不会自动执行命令。

```mermaid
flowchart LR
  Global[~/.walle/skills/*/SKILL.md] --> Manager[skill.Manager]
  Project[<workspace>/.walle/skills/*/SKILL.md] --> Manager
  Manager --> Agent[Agent system prompt injection]
  Manager --> Tool[skill.skill]
  Manager --> Cmd[/skill]
```

## 关键文件

| 文件 | 责任 |
| --- | --- |
| `internal/skill/skill.go` | `Skill`、`Manager`、目录加载、项目覆盖、reload、list/get。 |
| `internal/tools/skill_tool.go` | LLM 可调用的 skill 查询工具。 |
| `internal/commands/skill.go` | `/skill list/get/reload` 命令。 |

## 文件格式

```markdown
---
name: skill-name
description: 一句话描述
---
正文内容
```

- 全局来源：`~/.walle/skills/*/SKILL.md`。
- 项目来源：`<workspace>/.walle/skills/*/SKILL.md`。
- `name` 是索引键；后加载来源覆盖先加载来源，所以项目 skill 覆盖全局同名 skill。
- `Content` 保存 frontmatter 后的 Markdown 正文；`Scope` 和 `Path` 保留来源信息。

## 加载规则

- `NewManagerFromDirs(sources...)` 只保存有序来源。
- `LoadSkills` 用于启动；不存在目录和坏 skill 会跳过。
- `ReloadSkills` 用临时 Manager 完整加载，任意解析失败则当前快照不变。
- `ListSkills` 返回当前快照；`GetSkill` 按精确名称查找。
- `ensureConversationSetup` 只在空 context 时注入当前 skill snapshot；已写入 context 的 skill system message 不会被 reload 改写。

## 不要做

- 不在 Skill 层做任务 CRUD、任务状态、调度或持久化协议。
- 不把 skill 内容当命令自动执行。
- 不监听目录热更新；显式 `/skill reload` 才刷新。

## 验收

- 改解析：检查缺 frontmatter、缺 name、重复 name、项目覆盖全局。
- 改 reload：检查失败时旧 snapshot 保持不变。
- 改注入：检查新 context 和已有 context 的差异。
