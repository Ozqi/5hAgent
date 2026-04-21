# 5hAgent 开发进度

> 当前阶段：Phase 2 完成 ✅
> 代码量：3298 行（目标 < 4000 行）

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

- Manager实例复用
- 工具查找优化(map O(1)) // @claude这个真的有价值吗
- 代码review文档: doc/stage1_review.md

## Phase 2 完成 ✅（流式输出，工具扩展，上下文管理）

**P2.1 流式**:

- [x] Streaming输出（逐token显示）✅ 已实现基础版本
  - 修复多工具调用合并bug (commit d1dafcc)
  - [x] 边输出边执行工具 ✅ (commit 0a0b6c3)

**P2.2 工具扩展**:

- [x] glob: 文件模式匹配 ✅ (commit dd90511)
- [x] edit: 文件编辑（精确替换）✅ (commit dd90511)
- [x] 工具并发（只读工具并行执行）✅ (commit 7627673)

**P2.3 测试**:
为了实现SWE-bench 测试，我们新增了一些工具。

- [x] write_file: 创建/覆盖文件 ✅
- [x] grep: 代码搜索（ripgrep + grep fallback）✅
- [x] list_dir: 列出目录内容 ✅
- [ ] git_diff: 查看修改（P1）
- [ ] git_apply: 应用补丁（P1）
- [ ] 规范测试流程：去workspace目录(测试场)跑指定的SWE-BENCH，并生成测试报告。

**P2.4 上下文管理**:

- [x] 上下文压缩（超长对话处理）✅ (commit c5d4e0f)
  - 超过 50 条消息时自动压缩，保留最近 30 条
  - TODO: 更细节的压缩机制phase3实现
- [x] Skill注入机制 ✅
  - internal/skill/skill.go: 技能管理器
  - /skill list|enable|disable: 命令支持

---

## Phase 3 进行中（长程任务 + SWE-bench 工具补齐）

**P3.2 长程任务管理** ✅:

- [x] TaskList管理（持久化任务列表）✅
  - task_create: 创建任务
  - task_update: 更新任务状态
  - task_get: 获取任务详情
  - task_list: 列出所有任务
  - task_delete: 删除任务
  - 持久化到 .5hagent/tasks.json
- [ ] 自动规划（根据任务生成执行计划）
- [ ] 进度追踪（5h稳定工作）

---

## 参考

- Eino: https://github.com/cloudwego/eino
- Claude Code: /home/lzq/Proj/claude-code
