# TUI相关TODO

1. 【DONE】首先有一个叫TUI.md是干嘛用的提示词，在哪用的，我觉得这个大概率可以连带着使用他的代码一起清理掉。

   **确定结论**：这里实际文件是 `prompt/tui.md`，不是孤立废文件，不能清理。
   - 使用位置：`cmd/5hagent/main.go` 的 TUI 入口设置 `opts.PromptBase = "tui"`。
   - 加载链路：`runtime.New` -> `utils.LoadSystemPromptBase(promptDir, "tui", provider, model)`。
   - 作用：把 TUI 交互 prompt 和 headless/daemon 的 `prompt/main.md` 分开，避免普通 TUI 对话继承“先创建 task / 读 task.md”的无头任务行为。
   - 当前策略：保留 `prompt/tui.md`；如果用户目录旧安装缺失该文件，代码会回退到 `main.md`，但安装脚本应继续同步 `prompt/*.md`，避免长期缺失。


2. 【DONE】关于TUI的工具调用使用提示，”Ran XXX ...”
   1. 这个Ran前面的符号有点丑，需要换一个，我们换成类似字节跳动的钢琴键符号吧，如果在工作就调动，done了就停止。然后还没开始的就是灰色，动的时候是蓝色绿色；注意终端字符兼容性，注意终端字符兼容性，别用太小众的方式导致某些用户的终端环境渲染不出来
   2. 不需要展示Ran这仨字母
   3. 直接写调用的工具名吧
   4. 对于参数，截断再写...是对的， ，但是你有点过度截断了

   **本次确定实现**：
   - 使用兼容字符 `▮` 作为工具状态符号；running 用 spinner，done/error 用静态符号。
   - 去掉 `Ran/Running/Failed` 文案，标题直接显示工具名。
   - 参数摘要保留 `[...]`，单个参数截断从 80 放宽到 120，原始 JSON fallback 从 120 放宽到 180。
   - 暂不做复杂“钢琴键动画”组件；先保证兼容、稳定、可验收。

   **实现状态**：已实现为蓝色工具名和蓝色状态符号；error 仍保留红色。

3. 【DONE】每条消息都应该标明发出时间， worklog也要做到这一点，可能需要新增字段啥的

   **确定结论**：需要新增消息元数据或在持久化层补 `created_at`，不是纯渲染改动。当前 `Session` JSONL 只保存 role/content/tool_calls/tool_call_id/tool_name，缺少单消息时间字段；worklog 也需要统一事件时间。后续应作为独立提交实现，避免和本次工具展示改动混在一起。

   **实现状态**：已实现。Session JSONL 为每条消息写入 `created_at` 并同步到 `schema.Message.Extra`；TUI 历史/实时消息在首行右侧显示时间；headless worklog 的 Assistant、Tool Event、Status 段落标题带时间。

   时间戳的显示位置已调整为行内右侧。

4. 【DONE】我让你删掉“dir”的显示，是指删掉这三个字母，而不是不显示路径

   **实现状态**：已恢复显示当前路径，只删除 `dir` 文案前缀。
