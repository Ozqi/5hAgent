# miniAgent 开发进度

> 当前阶段：Phase 1 完成 ✅，准备进入 Phase 2
> 代码量：1072 行（目标 < 2000 行）

---

## Phase 1 ✅ (Agent Loop，工具调用)

**实现模块**:

- `internal/llm/client.go` (107行): LLM客户端，NewClientFromEnv(), NewClient()
- `internal/agent/agent.go` (260行): Agent核心，NewAgent(), Run(), exeTools(), findTool()
- `internal/tools/` (3个工具): read_file, exec_shell, registry
- `internal/context/ctx.go`: 上下文管理，Manager, Context
- `internal/cli/ui.go`: CLI输出
- `cmd/miniagent/main.go` (160行): 主入口，交互式循环

**核心流程**: main → Agent.Run() → ReAct循环(LLM生成 → 工具执行 → 结果回传)

**优化完成**:

- SystemPrompt自动注入
- Manager实例复用
- 工具查找优化(map O(1))
- 代码review文档: doc/stage1_review.md

## Phase 2 进行中（流式输出，工具扩展，上下文管理）

**P2.2 性能优化**:
- [x] Streaming输出（逐token显示）✅ 已实现基础版本
  - 修复多工具调用合并bug (commit d1dafcc)
  - TODO: 边输出边执行工具（参考 Claude Code StreamingToolExecutor）

**P2.1 工具扩展** (优先级高):
- [x] glob: 文件模式匹配 ✅
- [x] edit: 文件编辑（精确替换）✅
- [x] 工具并发（只读工具并行执行）✅

**P2.3 上下文管理** (当前任务):

- [ ] 上下文压缩（超长对话处理）← 当前任务
- [ ] Skill注入机制

---

## Phase 3 计划（长程任务）

- [ ] TaskList管理（持久化任务列表）
- [ ] 自动规划（根据任务生成执行计划）
- [ ] 进度追踪（5h稳定工作）

---

## 参考

- Eino: https://github.com/cloudwego/eino
- Claude Code: /home/lzq/Proj/claude-code
