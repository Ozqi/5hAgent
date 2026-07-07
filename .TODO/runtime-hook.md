# runtime-HOOK

一个新的feat，对于Agent runtime的每个动作（这个动作节点应该已经存在了，用于runtime向TUI展示打印的时候）。都可以加装HOOK，就是在”动作执行“的同时，后台可以运行一个脚本。

这个脚本的配置和摆放都在启动5hagent所在的项目路径的.5hagent里。

## 确定结论

- runtime hook 可以复用当前工具事件/运行事件思路，但首版不要覆盖“每个动作”；先只支持 tool start/tool end/tool error 这类已有稳定事件。
- 配置应放在项目 `.5hagent/hooks.*`，因为 hook 通常和项目工作流绑定；用户级全局 hook 后置。
- hook 脚本必须默认异步、限时、失败不阻塞主 Agent；否则会破坏 TUI/headless 的可预测性。
- hook 输入应是结构化 JSON，包括 event、tool、args_summary、result_summary、workspace、session_id。
- 不应让 hook 脚本直接修改 Agent 内存上下文；如需反馈，应写文件或通过后续明确的事件通道。
