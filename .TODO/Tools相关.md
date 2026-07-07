# 工具相关


## （待实现）工具调用统计

我希望能记录工具调用失败的情况。至少需要记录以下信息，用于判断可能的失败原因：

模型、调用参数；

对于失败次数过高的工具，后续会有一些手段处理：

- 改进工具的输入输出（不要直接返回traceback的报错，而是一串指导性的话告诉agent现在是啥情况）
- 为经常失败的某些模型，设置特定的前置提示词

### 确定结论

- 工具失败统计值得做，但不应先建复杂分析系统；先复用现有 `logger.ToolEvent` 链路记录结构化失败事件。
- 最小字段建议：time、session_id、model、tool_name、arguments_summary、error、workspace、task_id/process_id（有则填）。
- 原始参数可能包含敏感内容，默认只存摘要或截断值；完整参数仅在 debug/worklog 中按需保留。
- 统计落盘位置应区分层级：
  - 用户级汇总：`~/.5hAgent/tool-stats/`；
  - 项目执行证据：项目 `.5hagent/agents/<agent>/logs/`。
- 本条不和 TUI 工具展示混做，后续可作为独立 `feat(tools): record tool failure stats`。

## （设计中）动态工具注册

可以通过生成脚本（输入输出安全，有--help等等...），注册成工具，后续LLM可以直接从工具列表直接调用。

### 确定结论

- 动态工具注册必须走“显式目录 + schema 生成 + 人工审查/启用”，不能让 LLM 写完脚本就自动进入默认工具列表。
- 工具脚本至少需要：可执行入口、`--help`、输入 JSON schema、输出 JSON schema、权限声明。
- 推荐目录放在项目 `.5hagent/tools/`，用户级复用工具再考虑 `~/.5hAgent/tools/`。
- 首版只支持启动时加载，不做运行期热加载；这和当前 skill 生命周期固定的设计一致。
