# Skill Knowledge Spec

## 职责

知识层当前只管理可注入模型的 Skill。Skill 是启动或 reload 时从文件系统读取的提示词能力快照。

任务管理如有需要，由 Skill、MCP 或外置动态工具提供。

## 覆盖范围

| 路径 | 职责 |
| --- | --- |
| `internal/skill/skill.go` | Skill 数据结构、目录加载、项目覆盖、reload、list/get。 |

## 上游和下游

| 方向 | 模块 | 关系 |
| --- | --- | --- |
| 上游 | `internal/runtime` / `internal/agent` | 初始化 Manager，并把 skill snapshot 注入空 context。 |
| 上游 | `internal/tools/skill_tool.go` | 查询 skill list/get。 |
| 上游 | `internal/commands/skill.go` | 处理 TUI/daemon `/skill`。 |
| 下游 | 文件系统 | 读取用户级和项目级 `*/skills/*/SKILL.md`。 |

## Skill 接口

| 接口 | 输入 | 输出 | 行为 |
| --- | --- | --- | --- |
| `NewManagerFromDirs(sources...)` | 多来源目录 | `*Manager` | 保存有序来源，后加载覆盖先加载。 |
| `LoadSkills()` | 无 | error | 普通启动加载；跳过不存在目录和坏 skill。 |
| `ReloadSkills()` | 无 | error | 严格加载到临时 manager，成功后整体替换。 |
| `ListSkills()` | 无 | `[]*Skill` | 返回当前快照。 |
| `GetSkill(name)` | skill 名 | `*Skill,bool` | 按精确名称读取。 |

## 文件格式和覆盖

Skill 来源：

- 全局：`~/.walle/skills/*/SKILL.md`。
- 项目：`<project>/.walle/skills/*/SKILL.md`。

```markdown
---
name: skill-name
description: 一句话描述
---
正文内容
```

规则：

- 必须有 YAML frontmatter。
- `name` 是索引键；项目同名 skill 覆盖全局同名 skill。
- `Content` 保存 frontmatter 后的 Markdown 正文。
- `Scope` 记录 `global` 或 `project`；`Path` 记录实际路径。

## Reload 原子性

- `LoadSkills` 用于启动，普通坏文件会被跳过。
- `ReloadSkills` 用临时 Manager 完整加载所有来源。
- strict reload 中任意 skill 解析失败会返回 error，当前 manager 保持旧快照。
- reload 不重写已经注入当前 context 的 system skill message。

## 边界

- Skill Manager 持有启动或 reload 时的内存快照，不监听文件变化。
- Skill 内容只是 prompt 文本，不能自动当作命令执行。
- 固定知识层没有任务 CRUD、任务状态或任务持久化协议。
