# TUI相关TODO

1. [x] 内容块之间保留一行空白。实现：`76deeda7dc7599e22d7696c3e355ae3cd4239a04`。
2. [x] scroll 百分比放在元数据行最后。实现：`ce3b37848faa490d54ec26951daae76138fa70f1`。
3. [x] TUI 可通过 Unix Socket attach daemon Agent；`Ctrl+D` 或 `/detach` 只退出客户端，后台继续运行。实现：`b62e393a445310bdafc3dd0510e8ca138d086981`。
4. [x] TUI slash 命令支持 Tab 补全。实现：`23e28ccb041e68a770bd91e8efa1462a4c4f8a3f`。
5. [x] 按上箭头显示上一条提交内容。实现：`dc150761f76dcc1cb3922d7ddead0adea5f763e7`。

## CLI 查看和接入后台 Agent

设计想法：既然支持 detach/attach，就需要从命令行直接看见可接入的后台 Agent，并能选择目标连入。

- `5hagent ps`：列出当前 workspace 下正在运行或可接入的 Agent/Process，展示 target id、状态、任务/会话摘要、启动时间、最近输出或 worklog/report 路径。
- `5hagent attach <target>`：连入指定后台 Agent/TUI target。
- `5hagent attach` 进入选择模式时，Tab 应能提示可选 target；TUI 内 `/attach` 也应复用同一套 target 列表。
- target 命名要稳定、可读，优先复用 process id、task id、session id 或 tmux target，不要生成只能机器读的随机串。
- 这条只定义可见性和入口，不要求同时设计完整进程管理系统。

## 完成记录

- 后台 Agent 的 `ps`/`attach` 入口：`28284def42125b7a79d4c5fb220f697239765c7c`
- daemon/TUI 进程拆分、NDJSON IPC 与 detach/reattach：`b62e393a445310bdafc3dd0510e8ca138d086981`
