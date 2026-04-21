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
- `cmd/5hagent/main.go` (160行): 主入口，交互式循环

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
- [ ] webfetch

**P2.3 测试**:
为了实现SWE-bench 测试，我们新增了一些工具。

- [x] write_file: 创建/覆盖文件 ✅
- [x] grep: 代码搜索（ripgrep + grep fallback）✅
- [x] list_dir: 列出目录内容 ✅
- [ ] git_diff: 查看修改（P1）
- [ ] git_apply: 应用补丁（P1）
- [ ] 规范测试流程：去workspace目录(测试场)跑指定的SWE-BENCH，并生成测试报告。

**P2.4 上下文管理**:

- [x] 简单的上下文压缩 ✅ (commit c5d4e0f)
  - 超过 50 条消息时自动压缩，保留最近 30 条
  - TODO: 更细节的压缩机制phase3实现
- [x] Skill注入机制 ✅
  - internal/skill/skill.go: 技能管理器
  - .5hagent/skills/\*.json: 技能定义文件
  - /skill list|enable|disable: 命令支持

- [ ] TODO: SKILL的使用本身应该也封装成base tools, 作为一个命令就叫skilltool，然后斜杠命令单独开一个文件夹叫commonds,在这里实现斜杠命令, agent文件夹只存放Agent LOOP中涉及到的东西。
  - /skills 命令 = 用户手动浏览和管理 skills 的 UI 工具
  - SkillTool = LLM 自主调用 skills 的 API 接口

---

## Phase 3 进行中（长程任务 + 持久记忆）

**P3.1 长程任务管理** ✅:

- [x] TaskList管理（全局持久化任务列表）
  - task_create: 创建任务
  - task_update: 更新任务状态
  - task_get: 获取任务详情
  - task_list: 列出所有任务
  - task_delete: 删除任务
  - 持久化到 .5hagent/tasks.json
  - [] fix: 这些命令都作为tools了，这样会很乱，我希望他们同属于一个/task 命令下, 并且现在已有的task命令我也希望封装到tasktool里。就像skilltool一样。

- [ ] 自动规划（根据任务生成执行计划）
- [ ] 进度追踪（5h稳定工作）

- [ ]上下文压缩, compact文件夹, 实现精细的压缩管理
  - [ ] 工具调用可能返回超长的返回值，网络请求可能返回超大文件。

- [] history task
  - [ ] task完成以后，本身已经没用了，需要压缩并准备下个任务，task会被存档成history task并持久化成一个md文件。在原位置留下一段话，描述这个task做了啥，现在被存到了哪个位置。
  - [ ] 解压缩机制，如果后续发现前面有个记忆需要重新展开，从history task重新读取出来。

**P3.3 子任务 AgentTools** :

- [] AgentTools
  实现复杂任务分解，多 Agent 协作
- []
  Git worktree 支持。

**P3.4 子任务 AgentTools** :

**实现方式**: 不需要专门的 Memory 工具，通过 System Prompt 指令 + write_file 实现

```
System Prompt 添加:
You have a persistent memory system at `.5hagent/memory/`.
When you learn important information, save it using write_file:

---
name: user_role
description: User's expertise and preferences
type: user
---

Memory content...
```

**Memory 类型**:

- user: 用户角色、偏好、知识背景
- feedback: 用户反馈、纠正、确认的做法
- project: 项目进展、目标、截止日期
- reference: 外部资源引用

**关键点**:

- LLM 自主判断何时保存（prompt engineering）
- 复用 write_file 工具，无需新工具
- 文件格式：Markdown + YAML frontmatter

---

