# 5hAgent TodoList

> 规划原则：先易后难，先打地基，再接外部系统；先统一抽象和边界，再叠加自动化与长期记忆能力。

---

## Phase 3B 任务归档与压缩，待验收

### 3. `/compress` 手动压缩接入 `LMCOMPRESS`

优先级：高| 难度：中

状态：部分完成

**目标**

- 先不做自动压缩，只通过 `/compress` 手动触发压缩。
- 复用现有 `LMCOMPRESS` 能力。

**设计**

- 手动触发，不做自动判断
- 主上下文保留摘要和归档地址，完整内容归档到 `compact/`

**建议命令**
 
- `/compress `

**目录结构**

- `compact/messages/`
- `compact/tasks/`
- `compact/tools/`

**步骤**

- [x] 梳理现有 `LMCOMPRESS` 接口与调用方式
- [x] 定义 `/compress` 命令参数与子命令
- [x] 定义压缩产物的文件落盘位置
- [x] 定义压缩后主上下文中保留的摘要格式
- [ ] 接通任务压缩与工具输出压缩入口

**验收标准**

- 可显式压缩当前上下文
- 可压缩单个任务过程
- 可压缩大工具输出且不污染主上下文

---

### 4. HistoryTask 归档机制，待验收

优先级：高  
难度：中

状态：跳过（已有其他人在做）

**目标**

- 当任务完成后，将其从活动区移出，归档为可追溯、可恢复、可复用的历史任务。

**设计原则**

- HistoryTask 不是完整聊天记录 dump
- HistoryTask 是“任务完成摘要 + 关键决策 + 恢复入口”
- 第一阶段仅支持手动归档，不做自动归档

**触发时机**

- `status = completed`
- 当前不再需要持续推进
- 需要为主 `task.md` 腾出空间

**存储位置**

- 主文件：`task.md`
- 历史文件：`history/`
- 推荐命名：`history/YYYY-MM-DD-T001.md`

**`task.md` 保留形式**

- [x] 归档后不删除原任务，只保留瘦身条目
- [x] 条目中包含：
  - `status: archived`
  - `summary`
  - `history: history/...md`

**HistoryTask 模板**

- `title`
- `status`
- `archived_at`
- `owner`
- `priority`
- `Original Goal`
- `Outcome`
- `Key Decisions`
- `Files Touched`
- `Verification`
- `Follow-ups`
- `Recovery Hints`
- `Compressed Context`

**恢复机制**

- 支持从 `history/...md` 恢复回 active task
- 恢复后状态改为 `pending` 或 `in_progress`
- 保留 `restored_from` 来源信息

**建议命令**

- `/task archive <id>`
- `/task reopen <id>`
- `/task history`

**步骤**

- [x] 定义 HistoryTask 文件模板
- [x] 定义归档后的 `task.md` 摘要条目结构
- [x] 设计 `/task archive <id>`
- [x] 设计 `/task reopen <id>`
- [ ] 打通与 `/compress task <id>` 的关系

**验收标准**

- 已完成任务可以归档为 history 文件
- 主 `task.md` 仅保留摘要和指针
- 归档任务可恢复回 active 区继续推进

---

## Phase 3C 外部能力接入，待验收

### 5. MCP Client 设计

优先级：高  
难度：中到高

状态：进行中（foundation 已完成）

**目标**

- 先让 `5hAgent` 作为 MCP Client，连接外部 MCP 服务。

**设计**

- 支持外部 stdio MCP server
- 支持列出远程 tools
- 支持把远程 tools 注册到本地 tool registry
- 支持将 MCP 返回结果写回当前 Agent context

**配置模型**

- `name`
- `command`
- `args`
- `env`
- `startup_timeout`

**步骤**

- [x] 定义 MCP server 配置结构
- [ ] 定义 MCP client 生命周期
- [ ] 定义远程 tool schema 到本地 tool schema 的映射
- [x] 设计远程调用桥接层
- [ ] 定义连接失败与重试策略

**验收标准**

- 可成功连接一个 MCP server
- 可列出并调用其 tools
- 错误信息可追踪到具体 server/source

---

### 5.1 本地 Milvus + `claude-context` 验证

优先级：高  
难度：中到高

**目标**

- 在本地跑通 `claude-context` 的最小闭环，为后续正式接入提供验证环境。

**步骤**

- [ ] 本地部署 Milvus
- [ ] 跑通 `claude-context` MCP server
- [ ] 确定 embedding 方案：
  - [ ] 先 OpenAI
  - [ ] 或先 Ollama 本地 embedding
- [ ] 验证以下核心工具：
  - [ ] `index_codebase`
  - [ ] `search_code`
  - [ ] `clear_index`
  - [ ] `get_indexing_status`
- [ ] 用当前 `5hAgent` 仓库做索引与检索验证

**验收标准**

- 当前仓库可以被索引
- 可以执行语义搜索
- 索引状态与清理流程正常

---

### 5.2 5hAgent 接入 `claude-context`

优先级：高  
难度：高

**目标**

- 将 `claude-context` 提供的 MCP tools 纳入 `5hAgent` 的 ReAct 工具链。

**设计**

- MCP tools 通过 `mcp.claude_context.*` 命名暴露
- 通过 prompt 区分：
  - `search_code` 用于语义检索
  - `grep/glob/read_file` 用于精确定位与读取

**步骤**

- [ ] 把 `claude-context` tools 注册进本地 registry
- [ ] 定义系统提示词中的使用场景说明
- [ ] 验证 Agent 可自主调用 `search_code`
- [ ] 验证索引缺失时 Agent 可触发 `index_codebase`

**验收标准**

- `claude-context` 工具已成为 Agent 工具链一部分
- LLM 不会混淆本地搜索与语义搜索
- 检索结果能自然进入 ReAct 循环

---

## Phase 3D 智能化增强

### 8. 长期记忆 / 路书机制式自动新建SKILL

优先级：中  
难度：高

**目标**

- 沉淀高价值排障路径、代码定位路径和项目规则。

**设计**

- 记忆不是原始上下文堆积，而是高价值经验提炼
- 优先从 HistoryTask 中抽取关键决策与恢复提示

**步骤**

- [ ] 定义路书触发条件
- [ ] 定义路书模板结构
- [ ] 定义存储位置
- [ ] 定义检索和回注上下文的策略

**验收标准**

- 高代价定位路径可复用
- 下次同类任务能显著减少搜索轮数

---

### 9. 多 Agent / Worktree

优先级：最后  
难度：很高

**目标**

- 在基础设施成熟后，再做多 Agent 协作和 Git worktree 隔离。

**步骤**

- [ ] 定义子 Agent 职责边界
- [ ] 定义共享 tasklist 协议
- [ ] 定义 worktree 生命周期
- [ ] 定义结果回传与冲突处理

**验收标准**

- 多 Agent 不会互相污染工作区
- 用户能通过共享 tasklist 观察进度
- worktree 生命周期可控

---

## NewIdeaTODO

- [ ] 压缩模块测试和debug：压缩先依赖 `/compress` 手动触发，如果Task完成，自动触发压缩。
  1.  /compress测试就有问题
  2.  HistoryTask 待测试。
- [ ] 上下文压缩新思路：直接把上下文暴露给 LLM，让 LLM 可以自主裁减上下文，通过 `context.xxx` 实现上下文的增删改查。

- [ ] TUI 布局重做：删除左侧 sidebar 和右侧状态栏，保留其中有用的 Agent 运行状态信息。
  1.  状态信息集中展示在输入框上方和输入框下方。
  2.  输入框改成偏亮的灰色背景，和对话区形成清晰层级。
  3.  token 开销和输出速度（xx tokens/s）纳入输入框附近状态区，刷新频率按 1 秒设计。

- [ ]增加ReAct结束判断：没有工具调用了也不能退出。而是检查task区里，有没有未完成的任务。
- [ ] tool_use模块
      -[ ] feat: 增加tooluse的权限控制。主要是读写工具,读当前目录以外的东西需要批准,只读模式下,写命令需要批准.
      exeshell工具里,rm命令git命令,需要用正则的方式过滤并申请批准。
- [ ] feature：headless：无头模式，直接做task.md里未完成的任务，不与用户交互。
  1. 做完事以后更新md，如果有详细报告就整理进一个md文档里。并且在task.md里引用这个文档。
  2. task的格式需要更多属性，（当然不是必须填，可以为空），例如，涉及到的文件；git版本号；报告所在路径...
