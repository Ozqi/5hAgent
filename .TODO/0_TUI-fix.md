# TUI相关TODO

1. 行间距太小了，稍微改大一点，尤其是不同的块之间。
2. 这个 scroll 的进度百分比显示做得很好，我希望它在元数据行的最右侧。
3. TUI 能否 attach 到某个后台无头运行的 agent，并提供 `/detach` 退出命令。
4. 命令的 Tab 自动补全。
5. 按上箭头直接显示上一条命令。

## CLI 查看和接入后台 Agent

设计想法：既然支持 detach/attach，就需要从命令行直接看见可接入的后台 Agent，并能选择目标连入。

- `5hagent ps`：列出当前 workspace 下正在运行或可接入的 Agent/Process，展示 target id、状态、任务/会话摘要、启动时间、最近输出或 worklog/report 路径。
- `5hagent attach <target>`：连入指定后台 Agent/TUI target。
- `5hagent attach` 进入选择模式时，Tab 应能提示可选 target；TUI 内 `/attach` 也应复用同一套 target 列表。
- target 命名要稳定、可读，优先复用 process id、task id、session id 或 tmux target，不要生成只能机器读的随机串。
- 这条只定义可见性和入口，不要求同时设计完整进程管理系统。

## 完成记录

- 后台 Agent 的 `ps`/`attach` 入口：`28284def42125b7a79d4c5fb220f697239765c7c`
