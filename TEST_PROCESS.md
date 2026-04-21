# 5hAgent SWE-bench 测试流程

## 测试目标

验证 5hAgent 在真实代码修复任务上的能力，评估指标：
- ✅ 成功率：能否正确修复 bug
- ⏱️ 效率：完成任务的时间
- 🔄 轮数：需要多少轮对话
- 🛠️ 工具使用：调用了哪些工具

## 测试任务

从 SWE-bench 选取 3 个难度递增的任务：

| # | 任务ID | 难度 | 描述 | 预期修改 |
|---|--------|------|------|----------|
| 1 | astropy__astropy-14365 | 简单 | QDP 大小写问题 | 添加 `re.IGNORECASE` 标志 |
| 2 | astropy__astropy-6938 | 简单 | FITS D 指数 | 修复 `replace()` 赋值 |
| 3 | astropy__astropy-14182 | 中等 | RST header_rows | 添加参数支持 |

## 测试流程

### 自动化测试（推荐）

```bash
cd /home/lzq/Proj/5hAgent

# 测试单个任务
./test_swebench.sh 1  # 测试任务1
./test_swebench.sh 2  # 测试任务2
./test_swebench.sh 3  # 测试任务3

# 测试所有任务
./test_swebench.sh all
```

**脚本功能**：
1. 自动恢复任务到初始状态
2. 用预定义的 prompt 调用 5hAgent
3. 记录执行日志和 diff
4. 生成测试报告

**输出**：
- `test_results/<task_id>_<timestamp>.log` - 执行日志
- `test_results/<task_id>_<timestamp>.log.diff` - 代码修改
- `test_results/<task_id>_<timestamp>.log.status` - 状态（SUCCESS/FAILED）
- `test_results/report_<timestamp>.md` - 测试报告

### 手动测试

如果需要观察 Agent 的详细执行过程：

```bash
cd /home/lzq/Proj/5hAgent/workspace/astropy

# 启动 5hAgent
../../5hagent

# 输入任务描述
修复 astropy/io/ascii/qdp.py 中的 bug：在 _line_type 函数中，
找到 re.compile(_type_re) 这一行，添加 re.IGNORECASE 标志，
使正则表达式不区分大小写。
```

**观察要点**：
- Agent 是否正确理解任务
- 使用了哪些工具（read_file, grep, edit）
- 是否一次性完成还是需要多轮
- 是否有错误尝试

### Debug 模式

```bash
../../5hagent --debug
```

Debug 模式会显示：
- 所有 LLM API 调用的输入输出
- 工具调用的详细参数和返回值
- 上下文管理的压缩过程

## 验证修复

测试完成后验证修改是否正确：

```bash
cd workspace/astropy

# 查看修改
git diff astropy/io/ascii/qdp.py

# 运行单元测试（如果环境已配置）
python3 -m pytest astropy/io/ascii/tests/test_qdp.py::test_roundtrip -v
```

## 测试指标

记录以下数据：

### 基础指标
- **成功/失败**：是否正确修复
- **耗时**：从开始到完成的时间
- **轮数**：LLM 调用次数

### 工具使用
- read_file: 读取了哪些文件
- grep: 搜索了什么
- edit: 修改了几次
- exec_shell: 是否执行了测试

### 质量评估
- **准确性**：修改是否符合预期
- **效率**：是否有冗余操作
- **鲁棒性**：是否处理了边界情况

## 测试环境管理

### 恢复初始状态

```bash
cd workspace/astropy
git restore .
git clean -fd
```

### 查看当前状态

```bash
cd workspace/astropy
git status
git diff --stat
```

### 保存测试结果

```bash
# 保存成功的修改
git diff > ../../test_results/task1_success.patch

# 应用补丁
git apply ../../test_results/task1_success.patch
```

## 预期结果

### 任务 1: QDP 大小写

**修改位置**: `astropy/io/ascii/qdp.py:71`

```python
# 修改前
_line_type_re = re.compile(_type_re)

# 修改后
_line_type_re = re.compile(_type_re, re.IGNORECASE)
```

### 任务 2: FITS D 指数

**修改位置**: `astropy/io/fits/fitsrec.py:~1200`

```python
# 修改前
if 'D' in format:
    output_field.replace(encode_ascii('E'), encode_ascii('D'))

# 修改后
if 'D' in format:
    output_field = output_field.replace(encode_ascii('E'), encode_ascii('D'))
```

### 任务 3: RST header_rows

**修改位置**: `astropy/io/ascii/rst.py`

需要修改多处：
1. `__init__` 方法添加 `header_rows` 参数
2. `write` 方法处理 `header_rows`

## 持续改进

根据测试结果改进 Agent：

1. **失败分析**：为什么失败？缺少什么能力？
2. **工具优化**：是否需要新工具或改进现有工具？
3. **Prompt 优化**：系统提示词是否需要调整？
4. **上下文管理**：是否因为上下文压缩丢失了关键信息？

## 下一步

完成这 3 个任务后：
- 扩展到更多 SWE-bench 任务
- 测试更复杂的多文件修改
- 评估长程任务能力（需要多个步骤的任务）
