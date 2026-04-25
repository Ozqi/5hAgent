## Phase 3A 基础整理

### 1. Tool 二级分类

优先级：最高  
难度：低到中

状态：已完成

**目标**

- 统一 tools 的二级分类，避免本地工具与 MCP 工具重名或近名误调用。

**设计**

- 分类：
  - `base`
  - `task`
  - `skill`
  - `mcp`
- 元信息：
  - `category`
  - `source`
  - `display_name`
  - `full_name`
  - `original_name`
- 统一命名规则：
  - `base.read_file`
  - `base.grep`
  - `task.create` 或 `task` + `action`
  - `mcp.<server>.<tool>`

**步骤**

- [x] 定义 tool 分类模型
- [x] 定义 tool 元信息结构
- [x] 定义本地 tool 命名规范
- [x] 定义 MCP tool 前缀规则
- [x] 调整 registry，区分分组视图和执行视图
- [x] 更新系统提示词中的 tool 选择说明

**验收标准**

- 不同来源工具不会重名冲突
- LLM 能区分本地搜索与语义搜索
- MCP 工具可以按来源自动归组

---

### 2. Task 系统重构：`task.md` 单一真源

优先级：最高  
难度：中

状态：已完成

**目标**

- 以项目根目录 `task.md` 作为任务系统唯一来源。
- 用户与 Agent 共享同一个任务面板。

**设计**

- `task.md` 是唯一真源
- `/task` 命令与 `TaskTool` 共享同一底层 service
- 通过 prompt 约束 LLM 只能按模板修改 `task.md`
- 默认人工修改也遵守模板规则

**任务状态**

- `pending`
- `in_progress`
- `blocked`
- `completed`
- `archived`

**步骤**

- [x] 定义 task 数据模型
- [x] 定义 `task.md` 模板结构
- [x] 定义 task 解析与写回规则
- [x] 统一 `/task` 与 `TaskTool` 的底层接口
- [x] 设计 Agent 空闲时重新读取 `task.md` 的策略
- [x] 定义 LLM 修改 `task.md` 的约束提示词

**验收标准**

- 用户手改 `task.md` 后 Agent 可重新读取
- `/task` 与 `TaskTool` 语义一致
- Task 状态流转稳定并可持久化
---
