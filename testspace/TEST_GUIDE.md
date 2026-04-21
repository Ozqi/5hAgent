# 5hAgent SWE-bench 测试流程文档

## 概述

本测试流程用于评估 5hAgent 在真实代码修复任务上的表现。测试采用纯净环境，只提供 SWE-bench 原始问题描述，不提供任何额外提示或帮助。

## 目录结构

```
testspace/
├── swebench_original/          # 原始任务代码（只读，不修改）
│   ├── astropy/                # 任务1: QDP 大小写问题
│   ├── astropy-6938/           # 任务2: FITS D 指数问题
│   └── astropy-14182/          # 任务3: RST header_rows 支持
├── test_runner.sh              # 自动化测试运行器
├── results/                    # 测试结果目录（自动生成）
│   └── <task_id>_<timestamp>.json
└── <task_id>-workspace/        # 临时工作空间（每次测试创建）
    ├── agent.log               # 5hAgent 完整输出
    └── changes.diff            # git diff 结果（如果有修改）
```

## 测试原则

1. **纯净测试**: 只提供问题描述，不提供任何提示
2. **独立环境**: 每次从原始任务复制独立 workspace
3. **客观评估**: 通过 git diff 检查修改

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

## 任务列表

| # | 任务ID | 难度 | 目标文件 |
|---|--------|------|----------|
| 1 | astropy__astropy-14365 | 简单 | astropy/io/ascii/qdp.py |
| 2 | astropy__astropy-6938 | 简单 | astropy/io/fits/fitsrec.py |
| 3 | astropy__astropy-14182 | 中等 | astropy/io/ascii/rst.py |

## 查看结果

```bash
# 查看测试结果
ls -lt results/*.json

# 查看修改
cat <task_id>-workspace/changes.diff

# 查看日志
cat <task_id>-workspace/agent.log
```

## 测试流程

1. 从 swebench_original/ 复制任务到独立 workspace
2. 恢复初始状态（git restore + clean）
3. 运行 5hAgent（只输入问题描述）
4. 检查目标文件是否被修改
5. 生成 JSON 报告

## 结果判定

- **SUCCESS**: 目标文件有修改
- **NO_CHANGES**: 未修改目标文件
- **ERROR**: 执行失败
- **TIMEOUT**: 超过 300 秒

## 注意事项

1. 不要修改 swebench_original/（原始任务）
2. workspace 每次测试会被覆盖
3. 需要在父目录配置 .env 文件
4. API 限流时等待后重试
