# 功能扩展与最小 Runtime

一句话：Runtime 只启动 Agent 和执行工具；具体功能先放 Skill、脚本或工具组合里。

## 原则

- 新功能优先用现有 Skill 和工具组合完成。
- 重复且稳定的流程写脚本，用 `base.exec_shell` 调。
- 外部服务按需连接，不进默认启动链路。
- 两个以上真实入口都需要时，再考虑加入 Runtime。
- 不提前做插件系统、功能包、动态加载器和权限框架。

## TODO 分层

| 层级 | 文件 | 处理 |
| --- | --- | --- |
| 当前主线 | `11_上下文掩盖与LLM投影.md` / `10_动态终端工具.md` / `7_Runtime对外接口.md` | 和 walle 定位直接相关 |
| 待核验 | `验证skill加载.md` / `0_TUI-fix.md` / `lsp.md` | 有真实复现再做 |
| 暂缓 | `subAgent.md` / `思考.md` / `1_关于持久化.md` | 防止膨胀成框架 |

## Hook 边界

- `tool_start/tool_end/tool_error` 是事后事件通知。
- `prehook` 是工具执行前 gate。
- 第一版 prehook 只允许：继续、拒绝并给原因、返回提示给 Agent。
- 不做参数改写、完整权限系统和复杂生命周期。

## 验收

- 不使用该功能时，Runtime 启动开销不变。
- 删除 Skill 或脚本即可删除功能。
- `internal/runtime` 不出现具体业务功能名。
