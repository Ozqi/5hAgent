# Agent Systemd 验证

## 单测

```bash
go test ./internal/systemd ./internal/runtime ./internal/context ./internal/tools ./internal/agent
```

覆盖点：事件 dispatch、异步失败、runtime fake LLM、process report 命名。

## 构建

```bash
go build -o 5hagent ./cmd/5hagent
```

## 真实 workspace

真实 daemon/headless 验证统一在：

```text
/Users/bytedance/Proj/5hWorkSpace
```

## daemon smoke

```bash
cd /Users/bytedance/Proj/5hWorkSpace
/Users/bytedance/Proj/5hAgent/5hagent daemon --poll 1s
```

验收：stdout 出现 process start/completed/failed、task id、report path。

## worklog 审计

```bash
scripts/systemd_l4_audit.sh --mode no-tool <worklog>
scripts/systemd_l4_audit.sh --mode require-tool <worklog>
```

`no-tool` 要求 worklog 无 Tool Event；`require-tool` 要求出现目标工具事件。
