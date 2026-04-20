# SWE-bench 接入计划

## 目标
评估 5hAgent 在真实软件工程任务上的表现

## 当前能力评估

### 已有能力 ✅
- ReAct 循环（最多 10 轮）
- 工具调用：read_file, exec_shell, glob, edit
- 流式输出
- 上下文压缩（50 条消息 → 30 条）
- 工具并发执行（只读工具）

### 缺失能力 ❌
- write_file 工具（创建新文件）
- grep 工具（代码搜索）
- git 操作（checkout, diff, apply）
- 测试执行验证
- 长程任务规划（TaskList）

---

## 接入方案

### 阶段 1：补齐基础工具（1-2 天）

**优先级 P0**：
1. `write_file` - 创建/覆盖文件
2. `grep` - 代码搜索（基于 ripgrep）
3. `list_dir` - 列出目录内容

**优先级 P1**：
4. `git_diff` - 查看修改
5. `git_apply` - 应用补丁
6. `run_tests` - 执行测试

### 阶段 2：快速评估（1 天）

使用 **SWE-bench Lite**（300 个任务）的子集：
- 选择 10-20 个简单任务
- 手动运行 5hAgent
- 记录成功率和失败原因

**评估指标**：
- 任务完成率
- 平均轮数
- 工具调用次数
- 失败原因分类

### 阶段 3：自动化评估（2-3 天）

实现 SWE-bench 适配器：

```go
// internal/benchmark/swebench.go
type SWEBenchAdapter struct {
    agent *agent.Agent
}

func (a *SWEBenchAdapter) SolveIssue(issue *Issue) (*Solution, error) {
    // 1. 设置环境（clone repo, checkout commit）
    // 2. 构造 prompt（issue description + repo context）
    // 3. 运行 agent
    // 4. 提取 patch
    // 5. 验证测试
}
```

### 阶段 4：长程任务支持（Phase 3）

- TaskList 管理
- 自动规划
- 进度追踪
- 支持 5h+ 长程任务

---

## 快速开始

### 方法 1：手动测试（最快）

1. 从 SWE-bench Lite 选一个简单任务
2. 手动准备环境
3. 用 5hAgent 交互式解决
4. 记录过程和结果

### 方法 2：使用 swekit

```bash
# 安装 swekit
pip install swekit

# 创建适配器脚本
# scripts/swebench_adapter.py
```

### 方法 3：参考 SWE-agent

克隆官方实现，学习其工具设计：
```bash
git clone https://github.com/princeton-nlp/SWE-agent
```

---

## 预期结果

**短期目标**（补齐工具后）：
- SWE-bench Lite: 5-10% 成功率
- 主要瓶颈：规划能力、上下文理解

**中期目标**（Phase 3 完成后）：
- SWE-bench Lite: 15-25% 成功率
- 支持多步骤任务

**长期目标**：
- SWE-bench Verified: 10-20% 成功率
- 与开源 agent 对比

---

## 参考资源

- [SWE-bench 官方](https://www.swebench.com/)
- [SWE-agent 文档](https://swe-agent.com/latest/)
- [swekit PyPI](https://pypi.org/project/swekit/)
- [SWE-bench Leaderboard](https://www.vals.ai/benchmarks/swebench)
