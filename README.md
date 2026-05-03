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
│   ├── tool_use.go       # ToolCall 解析与工具执行
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

## 安装

### 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/Ozqi/5hAgent/master/install.sh | bash
```

或克隆后手动执行：

```bash
git clone https://github.com/Ozqi/5hAgent.git
cd 5hAgent
bash install.sh
```

### 手动安装

```bash
# 1. 编译
go build -o 5hagent cmd/5hagent/main.go

# 2. 编辑配置文件，设置 API Key
vim ~/.5hAgent/config.yaml

# 3. 运行
./5hagent

# Debug 模式
./5hagent --debug
```

## 配置

配置文件位于 `~/.5hAgent/config.yaml`，首次运行时自动创建。

### 基础配置

```yaml
llm:
  api_key: your_api_key_here # 必需：API Key
  base_url: https://api.anthropic.com # LLM API 地址
  model: claude-sonnet-4-6 # 模型名称
  max_tokens: 4096 # 最大 token 数

agent:
  name: 5hAgent
  max_total_tokens: 200000 # 会话总 token 上限
  repeat_tool_limit: 5 # 工具重复调用上限
  debug: false # 调试模式
```

### MCP 服务器配置

支持通过 MCP (Model Context Protocol) 集成外部工具服务器：

```yaml
mcp:
  servers:
    - name: filesystem
      command: npx
      args:
        - -y
        - @modelcontextprotocol/server-filesystem
        - /tmp
      startup_timeout: 10s
```

更多 MCP 配置见 [doc/mcp.md](doc/mcp.md)

### 配置优先级

配置加载优先级：`~/.5hAgent/config.yaml` > `.env` > 默认值

向后兼容：仍支持项目根目录的 `.env` 文件（用于快速测试）

### 配置文件位置

- 配置文件：`~/.5hAgent/config.yaml`
- 技能目录：`~/.5hAgent/skills/`
- 任务持久化：`./.5hagent/task.md`

## 参考

- [Eino GitHub](https://github.com/cloudwego/eino)
- [Eino 文档](https://www.cloudwego.io/docs/eino/overview/)
- Claude Code 源码：`/home/lzq/Proj/claude-code`
