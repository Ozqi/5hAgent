<!-- 项目介绍，如何安装和使用项目 -->

# 5hAgent

> 此文档由鄙人手工编写，没有烦人的emoji表情。

花5h实现一个能稳定工作5h的Agent。

## 目标

- 核心循环完整可用（Phase 1 ✅）
- 参考 Claude Code 设计，逐步演进
- 代码简洁（< 2000 行）

## 技术栈

- **语言**: Go 1.23+
- **Agent 框架**: [Eino](https://github.com/cloudwego/eino) (字节跳动开源)
- **LLM Provider**: Claude API (兼容格式)
- **CLI**: Cobra + readline

## 架构

```
internal/
├── agent/agent.go      # Agent 核心：NewAgent(), Run(), exeTools()
├── llm/client.go       # LLM 客户端：NewClientFromEnv(), GetModel()
├── tools/              # 工具系统：read_file, exec_shell, registry
├── context/ctx.go      # 上下文管理：Manager, Context
└── cli/ui.go           # CLI 输出
cmd/miniagent/main.go   # 主入口：交互式循环
```

**关键流程**: `main.go` → `Agent.Run()` → ReAct 循环（LLM 生成 → 工具执行 → 结果回传）

## 运行

```bash
# 1. 配置 .env 文件
cp .env.example .env
# 编辑 .env，设置 CLAUDE_API_KEY

# 2. 运行
go run cmd/miniagent/main.go

# 或编译后运行
go build -o miniagent cmd/miniagent/main.go
./miniagent

# Debug 模式
./miniagent --debug
```

## 已完成（Phase 1）

- ✅ 项目初始化（Go 1.23 + Eino）
- ✅ LLM 客户端（Claude API 格式）
- ✅ 基础工具（read_file, exec_shell）
- ✅ Agent 核心（ReAct 循环，最多 10 轮）
- ✅ 交互式 CLI（readline + 历史）
- ✅ 上下文管理（消息历史）

**代码量**: 1072 行

## 下一步（Phase 2）

- [ ] 工具扩展（glob, edit, web_fetch）
- [ ] Streaming 输出
- [ ] 工具并发执行（只读并行）
- [ ] 上下文压缩
- [ ] Skill 注入机制

## 参考

- [Eino GitHub](https://github.com/cloudwego/eino)
- [Eino 文档](https://www.cloudwego.io/docs/eino/overview/)
- Claude Code 源码：`/home/lzq/Proj/claude-code`
