# Skill

## 职责

`internal/skill` 在 Agent 启动时加载 skill 快照，并在空 context 初始化时注入已加载的 skills。

## 路径

| 来源 | 路径 | 优先级 |
| --- | --- | --- |
| 全局 | `~/.walle/skills/*/SKILL.md` | 低 |
| 项目 | `<project>/.walle/skills/*/SKILL.md` | 高 |

项目同名 skill 覆盖全局 skill。

## 文件

| 文件 | 作用 |
| --- | --- |
| [skill.go](../../internal/skill/skill.go) | 读取、合并和来源信息 |
| [skill_tool.go](../../internal/tools/skill_tool.go) | `skill.skill` 只读工具 |
| [skill.go](../../internal/commands/skill.go) | `/skill` 命令 |
| [agent.go](../../internal/agent/agent.go) | `injectSkills` 注入 system message |

## 生命周期

- 启动时加载。
- Agent 生命周期内不热加载。
- 已加载 skill 作为独立 system message 注入。
- `/skill list|get` 读取当前 skill manager 快照。
- `/skill reload` 原子重扫磁盘；失败保留旧快照，成功后只影响后续查询和新 context，不替换当前 context 已注入的 skill message。
