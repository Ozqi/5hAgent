<!-- 项目介绍，如何安装和使用项目 -->

# 5hAgent

> 基于 Go + Eino 的轻量级 AI Agent，代码量 ~3300 行

## 目标

- 核心循环完整可用（Phase 1 ✅）
- 流式输出、工具扩展、上下文管理（Phase 2 ✅）
- 长程任务管理、SWE-bench 工具补齐（Phase 3 进行中）
- 参考 Claude Code 设计，逐步演进
- 代码简洁（< 4000 行）

## 技术栈

- **语言**: Go 1.21+
- **Agent 框架**: [Eino](https://github.com/cloudwego/eino) (字节跳动开源)
- **LLM Provider**: Claude API 兼容格式（代码默认 `https://api.anthropic.com`）
- **CLI**: Cobra + readline

## 架构

```
internal/
├── agent/
│   ├── agent.go          # Agent 核心：ReAct 循环、流式输出、skill 注入
│   ├── tool_executor.go  # 工具并发/串行调度
│   └── tasklist.go       # 任务持久化
├── llm/client.go       # LLM 客户端
├── tools/              # 工具系统
│   ├── read_file.go    # 文件读取
│   ├── write_file.go   # 文件写入
│   ├── edit.go         # 文件编辑
│   ├── glob.go         # 文件匹配
│   ├── grep.go         # 代码搜索
│   ├── list_dir.go     # 目录列表
│   ├── exec_shell.go   # Shell 执行
│   ├── task_tool.go    # 统一 task 工具
│   ├── skill_tool.go   # 统一 skill 工具
│   └── registry.go     # 工具注册
├── skill/skill.go      # 技能管理器
├── context/ctx.go      # 上下文管理、自动压缩
├── commands/           # /skill /task 命令
├── logger/logger.go    # 日志系统
└── cli/ui.go           # CLI 输出
cmd/5hagent/main.go   # 主入口
```

**关键流程**: `main.go` → `Agent.RunStream()` → ReAct 循环（LLM 流式生成 → 工具并发执行 → 结果回传）

## 运行

```bash
# 1. 配置 .env 文件
cp .env.example .env
# 编辑 .env，设置 API_KEY

# 2. 运行
go run cmd/5hagent/main.go

# 或编译后运行
go build -o 5hagent cmd/5hagent/main.go
./5hagent

# Debug 模式
./5hagent --debug
```

## 功能特性

### Phase 1 ✅（Agent Loop，工具调用）

- ✅ LLM 客户端（Claude API 格式）
- ✅ 基础工具（read_file, exec_shell）
- ✅ Agent 核心（ReAct 循环，最多 10 轮）
- ✅ 交互式 CLI（readline + 历史）
- ✅ 上下文管理（消息历史）

**代码量**: 1072 行

### Phase 2 ✅（流式输出，工具扩展，上下文管理）

**P2.1 流式输出**:
- ✅ Streaming 输出（逐 token 显示）
- ✅ 多工具调用合并修复
- ✅ 边输出边执行工具（异步执行）

**P2.2 工具扩展**:
- ✅ glob: 文件模式匹配
- ✅ edit: 文件编辑（精确替换）
- ✅ 工具并发执行（只读工具并行）

**P2.3 SWE-bench 工具**:
- ✅ write_file: 创建/覆盖文件
- ✅ grep: 代码搜索（ripgrep + grep fallback）
- ✅ list_dir: 列出目录内容

**P2.4 上下文管理**:
- ✅ 上下文自动压缩（50→30 条消息）
- ✅ Skill 注入机制（独立消息注入）
- ✅ /skill list|enable|disable 命令

**代码量**: 3320 行

### Phase 3 进行中（长程任务 + SWE-bench 工具补齐）

**TaskList 管理系统** ✅:
- ✅ 任务 CRUD 操作（create, update, get, list, delete）
- ✅ 持久化到 `.5hagent/tasks.json`
- ✅ 并发安全（sync.RWMutex）
- ✅ 进度统计

**待完成**:
- [ ] 自动规划（根据任务生成执行计划）
- [ ] 进度追踪（5h 稳定工作）
- [ ] git_diff, git_apply 工具
- [ ] SWE-bench 测试流程

**代码量**: 3359 行

## 工具清单

| 类型 | 工具 | 功能 | 并发 |
|------|------|------|------|
| 文件操作 | read_file, write_file, edit | 读写编辑文件 | 读✅ 写❌ |
| 文件搜索 | glob, grep, list_dir | 文件匹配、代码搜索、目录列表 | ✅ |
| 执行 | exec_shell | Shell 命令执行 | ❌ |
| Agent 工具 | task, skill | 任务管理、skill 管理 | `task get/list` 并发，其余串行 |

**当前注册总计**: 9 个工具

## Skill 系统

支持动态加载和启用技能提示词，增强 Agent 在特定场景下的能力。

**格式**: Markdown + YAML frontmatter（遵循 Claude Code 规范）

**加载位置**: `.5hagent/skills/*/SKILL.md`

**使用方式**:
```bash
/skill list                  # 列出所有技能
/skill enable using-superpowers
/skill disable writing-plans
```

**当前仓库内已有技能示例**:
- `using-superpowers`
- `brainstorming`
- `writing-plans`
- `test-driven-development`
- `systematic-debugging`
- `verification-before-completion`

**创建新技能**:
```bash
mkdir -p .5hagent/skills/my-skill
cat > .5hagent/skills/my-skill/SKILL.md << 'EOF'
---
name: my-skill
description: Use when user asks to "trigger phrase".
---

# My Skill

Skill content here...
EOF
```

详见 `doc/skill_injection.md`

## 下一步

- [ ] 自动规划（根据大任务生成子任务）
- [ ] 进度追踪（5h+ 长程任务稳定执行）
- [ ] git_diff, git_apply 工具
- [ ] SWE-bench 评估流程
- [ ] 自我内化（学习流程，生成 skill）

## 文档

- `doc/agent.md` - Agent 核心模块
- `doc/tools.md` - 工具系统
- `doc/context.md` - 上下文管理
- `doc/skill_injection.md` - Skill 注入机制
- `doc/prompt.md` - Prompt 加载
- `doc/commonds.md` - `/skill` 和 `/task` 命令

## 参考

- [Eino GitHub](https://github.com/cloudwego/eino)
- [Eino 文档](https://www.cloudwego.io/docs/eino/overview/)
- Claude Code 源码：`/home/lzq/Proj/claude-code`
