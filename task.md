# miniAgent 开发进度

> 当前阶段：Phase 3 TaskList 完成 ✅
> 代码量：3015 行（目标 < 4000 行）

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

## Phase 2 基本完成 ✅（流式输出，工具扩展，上下文管理）

**P2.2 性能优化**:

- [x] Streaming输出（逐token显示）✅ 已实现基础版本
  - 修复多工具调用合并bug (commit d1dafcc)
  - TODO: 边输出边执行工具（参考 Claude Code StreamingToolExecutor）

**P2.1 工具扩展**:

- [x] glob: 文件模式匹配 ✅ (commit dd90511)
- [x] edit: 文件编辑（精确替换）✅ (commit dd90511)
- [x] 工具并发（只读工具并行执行）✅ (commit 7627673)

**P2.3 上下文管理**:

- [x] 上下文压缩（超长对话处理）✅ (commit c5d4e0f)
  - 超过 50 条消息时自动压缩，保留最近 30 条
- [ ] Skill注入机制（暂缓，Phase 3 实现）

---

## Phase 3 进行中（长程任务 + SWE-bench 工具补齐）

**P3.1 SWE-bench 基础工具** ✅:

- [x] write_file: 创建/覆盖文件 ✅
- [x] grep: 代码搜索（ripgrep + grep fallback）✅
- [x] list_dir: 列出目录内容 ✅
- [ ] git_diff: 查看修改（P1）
- [ ] git_apply: 应用补丁（P1）

**P3.2 长程任务管理** ✅:

- [x] TaskList管理（持久化任务列表）✅
  - task_create: 创建任务
  - task_update: 更新任务状态
  - task_get: 获取任务详情
  - task_list: 列出所有任务
  - task_delete: 删除任务
  - 持久化到 .miniagent/tasks.json
- [ ] 自动规划（根据任务生成执行计划）
- [ ] 进度追踪（5h稳定工作）

---

## 参考

- Eino: https://github.com/cloudwego/eino
- Claude Code: /home/lzq/Proj/claude-code
