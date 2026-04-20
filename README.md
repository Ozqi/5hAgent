<!-- 项目介绍，如何安装和使用项目 -->

# 5hAgent

> 基于 Go + Eino 的轻量级 AI Agent，代码量 ~3000 行

## 目标

- 核心循环完整可用（Phase 1 ✅）
- 流式输出、工具扩展、上下文管理（Phase 2 ✅）
- 长程任务管理、SWE-bench 工具补齐（Phase 3 ✅）
- 参考 Claude Code 设计，逐步演进
- 代码简洁（< 4000 行）

## 技术栈

- **语言**: Go 1.21+
- **Agent 框架**: [Eino](https://github.com/cloudwego/eino) (字节跳动开源)
- **LLM Provider**: Claude API 兼容格式（默认 MiniMax）
- **CLI**: Cobra + readline

## 架构

```
internal/
├── agent/agent.go      # Agent 核心：ReAct 循环、流式输出、工具并发
├── llm/client.go       # LLM 客户端
├── tools/              # 工具系统（12 个工具）
│   ├── read_file.go    # 文件读取
│   ├── write_file.go   # 文件写入
│   ├── edit.go         # 文件编辑
│   ├── glob.go         # 文件匹配
│   ├── grep.go         # 代码搜索
│   ├── list_dir.go     # 目录列表
│   ├── exec_shell.go   # Shell 执行
│   ├── task_tools.go   # 任务管理（5 个工具）
│   └── registry.go     # 工具注册
├── tasklist/           # 任务列表管理
├── context/ctx.go      # 上下文管理、自动压缩
├── logger/logger.go    # 日志系统
└── cli/ui.go           # CLI 输出
cmd/miniagent/main.go   # 主入口
```

**关键流程**: `main.go` → `Agent.RunStream()` → ReAct 循环（LLM 流式生成 → 工具并发执行 → 结果回传）

## 运行

```bash
# 1. 配置 .env 文件
cp .env.example .env
# 编辑 .env，设置 API_KEY

# 2. 运行
go run cmd/miniagent/main.go

# 或编译后运行
go build -o miniagent cmd/miniagent/main.go
./miniagent

# Debug 模式
./miniagent --debug
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

- ✅ Streaming 输出（逐 token 显示）
- ✅ 多工具调用合并修复
- ✅ 工具扩展（glob, edit）
- ✅ 工具并发执行（只读工具并行）
- ✅ 上下文自动压缩（50→30 条消息）

**代码量**: 2099 行

### Phase 3 ✅（长程任务 + SWE-bench 工具补齐）

**SWE-bench P0 工具集**:
- ✅ write_file: 创建/覆盖文件
- ✅ grep: 代码搜索（ripgrep + grep fallback）
- ✅ list_dir: 列出目录内容

**TaskList 管理系统**:
- ✅ 任务 CRUD 操作（create, update, get, list, delete）
- ✅ 持久化到 `.5hagent/tasks.json`
- ✅ 并发安全（sync.RWMutex）
- ✅ 进度统计

**代码量**: 3073 行

**详细文档**: `doc/phase3_summary.md`

## 工具清单

| 类型 | 工具 | 功能 | 并发 |
|------|------|------|------|
| 文件操作 | read_file, write_file, edit | 读写编辑文件 | 读✅ 写❌ |
| 文件搜索 | glob, grep, list_dir | 文件匹配、代码搜索、目录列表 | ✅ |
| 执行 | exec_shell | Shell 命令执行 | ❌ |
| 任务管理 | task_create, task_update, task_get, task_list, task_delete | 任务 CRUD | 读✅ 写❌ |

**总计**: 12 个工具（6 个只读支持并发，6 个写入串行执行）

## 下一步（Phase 3+）

- [ ] 自动规划（根据大任务生成子任务）
- [ ] 进度追踪（5h+ 长程任务稳定执行）
- [ ] SWE-bench P1 工具（git_diff, git_apply, run_tests）
- [ ] SWE-bench 评估
- [ ] 自我内化（学习流程，生成 skill）

## 文档

- `doc/agent.md` - Agent 核心模块
- `doc/tools.md` - 工具系统
- `doc/tasklist.md` - TaskList 管理
- `doc/context.md` - 上下文管理
- `doc/phase2_summary.md` - Phase 2 总结
- `doc/phase3_summary.md` - Phase 3 总结
- `doc/swe_bench_integration.md` - SWE-bench 接入计划

## 参考

- [Eino GitHub](https://github.com/cloudwego/eino)
- [Eino 文档](https://www.cloudwego.io/docs/eino/overview/)
- Claude Code 源码：`/home/lzq/Proj/claude-code`
