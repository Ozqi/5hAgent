# SWE-bench 测试空间

## 目录结构

```
testspace/
├── swebench_original/          # 原始任务（只读）
│   ├── astropy/
│   ├── astropy-6938/
│   └── astropy-14182/
├── test_runner.sh              # 测试运行器
├── results/                    # 测试结果
└── <task_id>-workspace/        # 临时工作空间
```

## 使用方法

```bash
cd /home/lzq/Proj/5hAgent/testspace

# 测试单个任务
./test_runner.sh 1
./test_runner.sh 2
./test_runner.sh 3

# 测试所有任务
./test_runner.sh all
```

## 测试原则

1. **纯净测试**: 只提供问题描述，不提供任何提示
2. **独立环境**: 每次从原始任务复制
3. **客观评估**: 通过 git diff 检查修改

## 任务列表

| # | 任务ID | 难度 |
|---|--------|------|
| 1 | astropy__astropy-14365 | 简单 |
| 2 | astropy__astropy-6938 | 简单 |
| 3 | astropy__astropy-14182 | 中等 |

## 查看结果

```bash
# 查看测试结果
ls -lt results/*.json

# 查看修改
cat <task_id>-workspace/changes.diff

# 查看日志
cat <task_id>-workspace/agent.log
```
