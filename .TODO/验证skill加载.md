# Skill 按需加载

## 当前事实

- `internal/skill.Manager.LoadSkills()` 会读取完整 `SKILL.md`。
- `injectSkills` 会把已加载 Skill 的完整内容注入 system message。
- `/skill reload` 已支持原子重载，来源和覆盖规则见 `.spec/knowledge.md`。

## 目标

启动时只向模型注入 Skill 的 `name`、`description` 和触发说明；模型命中后再通过 `skill.skill get` 读取完整内容。

## 待设计

- [ ] 明确 Skill 描述索引的 system prompt 格式。
- [ ] 明确同一 Context 中 Skill 全文只注入一次的标记位置。
- [ ] 明确 `/skill reload` 后已注入 Skill 的版本语义。
- [ ] 设计最小验收：模型根据 description 主动调用 `skill.skill get`，再按正文执行。

## 边界

- 不做后台文件监听。
- 不新增插件系统。
- 不让解析失败破坏当前可用快照。
