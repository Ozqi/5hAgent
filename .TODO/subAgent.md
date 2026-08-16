# sudoAgent && subAgent

subagent应该由Agent process发出和管理，作为“线程” Agent thread；他由Agent precess管理，应该和共享相同的上下文。（开销问题）

sudoAgent的作用：例如codex的/goal命令（如果不知道你就上网搜），大概率是这样做的利用一个子agent，在React循环快结束的时候去判断，这个goal有没有达成，这个时候新起的这个决策agent，在我们这里叫sudo Agent，我们后续也会有很多这种在调度层面，需要偶尔”一次性调用“的进行决策的Agent

## 确定结论

- subAgent 和 sudoAgent 是两类能力，不应混在同一个实现里。
- subAgent 更像由 AgentProcess 管理的工作线程：需要明确输入、共享/隔离上下文策略、结果回传和生命周期。
- sudoAgent 更像一次性 decision caller：只拿有限上下文做判断，返回结构化决策，不拥有长期上下文，也不直接执行工具。
- 当前最小 systemd/runtime 不应先实现共享上下文 subAgent；共享上下文会带来 token 成本、写入冲突和审计困难。
- 后续优先实现 sudoAgent/decision 这类“一次性判断”能力，因为边界更小：输入快照 -> 输出 JSON 决策 -> 调度层执行或拒绝。
