# 本地 Ollama Ornith 模型计划

## 背景

当前本机和项目状态：

- 机器：MacBook Pro，Apple M4 Pro，48GB 统一内存。
- Ollama：已安装，当前版本 `0.32.1`。
- 已有模型：`ornith:9b`，大小约 5.6GB。
- 项目接入方式：现有 `LLM_MODEL=ollama/<model>` + `LLM_OLLAMA_FORMAT=openai` + `LLM_OLLAMA_BASE_URL=http://localhost:11434/v1` 已覆盖本地 Ollama。

用户提出“onirth 25B A3B”。联网核验后，公开主线信息更像是 `Ornith 35B-A3B`：

- Ollama 官方库存在 `ornith:35b`，约 21GB，256K context。
- Ollama 官方库存在 `ornith-1.5:35b`，约 23GB，256K context，Q4_K_M。
- Ornith 1.5 官方模型卡描述为 35B MoE，约 3B active parameters，定位 agentic coding。
- 未查到稳定的 Ollama 官方 `25B A3B` 标签；本计划先按“35B-A3B”处理，`25B` 作为待核验关键词保留。

## 目标

新增一个可人工执行的本地模型验证计划，目标是选出当前 Mac 上最值得接入 walle 的 Ornith 版本。

计划只覆盖：

1. 选型：确认拉取哪个 Ollama tag。
2. 配置：确认 walle 使用的 `LLM_MODEL` 写法。
3. 验证：用最小真实任务检查工具调用、响应速度和卡顿情况。
4. 回退：保留 `ornith:9b` 作为轻量 fallback。

暂不改：

- 不新增 provider 抽象。
- 不改 runtime 架构。
- 不改工具系统。
- 不写自动下载器。
- 不把模型选择做成复杂 benchmark 框架。

## 版本候选

| 候选 | 来源 | 大小 | 适配判断 | 用途 |
| --- | --- | ---: | --- | --- |
| `ornith:9b` | Ollama 官方 | 约 5.6GB | 已安装，稳定轻量 | 默认 fallback、快速 TUI/工具调用检查 |
| `ornith:35b` | Ollama 官方 | 约 21GB | 48GB Mac 可尝试，质量高于 9B | 第一轮大模型候选 |
| `ornith-1.5:35b` | Ollama 官方 | 约 23GB | 48GB Mac 可尝试，更新、更偏 agentic coding | 推荐主候选 |
| `hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:AD-Q5_K-Q4_K` | Hugging Face / Ollama 直拉 | 约 22.1GB | 比官方 Q4 更准，体积接近 | 推荐高性价比候选 |
| `hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:Q5_K_M` | Hugging Face / Ollama 直拉 | 约 24.7GB | 高于 Q4，48GB Mac 可试 | 常规高质量候选 |
| `hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:AD-Q6_K` | Hugging Face / Ollama 直拉 | 约 29.1GB | 明显高于 Q4，内存压力更大 | 48GB Mac 的质量优先候选 |
| `hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:Q8_0` | Hugging Face / Ollama 直拉 | 约 36.9GB | 质量最高，留给短上下文试验 | 不建议日常默认 |
| `hf.co/ornith-ai/Ornith-1.5-35B-A3B-GGUF:Q5_K_M` | Hugging Face / Ollama 直拉 | 约 25.3GB | 官方 GGUF Q5 | 官方来源优先时使用 |
| `hf.co/ornith-ai/Ornith-1.5-35B-A3B-GGUF:Q6_K` | Hugging Face / Ollama 直拉 | 约 29.2GB | 官方 GGUF Q6 | 官方来源质量优先时使用 |
| `ornith-1.5:397b` | Ollama 官方 | 约 242GB | 当前机器不适合 | 不纳入本地计划 |

## 当前推荐

第一阶段推荐：`ornith-1.5:35b`。

理由：

- 官方 Ollama tag，安装和配置最少。
- Q4_K_M，约 23GB，适合 48GB 统一内存先试。
- Ornith 1.5 相比 1.0 更贴近 agentic coding 和工具调用场景。
- walle 已经能通过 OpenAI-compatible endpoint 调 Ollama，接入成本低。

如果想要比 Q4 更好的版本，推荐顺序：

1. `hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:AD-Q5_K-Q4_K`：大小约 22.1GB，公开表格显示 Top-1 约 93.52%，比 stock `Q5_K_M` 略好且更小，适合直接替代官方 Q4。
2. `hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:AD-Q6_K`：大小约 29.1GB，Top-1 约 95.31%，接近 `Q8_0`，适合 48GB Mac 做质量优先运行。
3. `hf.co/ornith-ai/Ornith-1.5-35B-A3B-GGUF:Q6_K`：官方 GGUF，约 29.2GB；来源更正统，指标略弱于 AtomicChat 的动态量化表。
4. `Q8_0`：约 36.9GB；48GB 机器可短上下文尝试，日常 TUI/Agent 任务容易挤压系统和 KV cache 空间。

当前机器是 48GB 统一内存。按“先能跑稳，再追质量”的顺序，建议先运行 `AD-Q5_K-Q4_K`，稳定后再试 `AD-Q6_K`。

保守 fallback：`ornith:35b`。

- 若 `ornith-1.5:35b` 的工具调用、模板或输出风格不稳定，回退到 `ornith:35b` 做对照。
- 若 35B 系列导致 TUI 卡顿、系统内存压力高或工具调用错误率高，回退到现有 `ornith:9b`。

## 执行步骤

### 1. 拉取模型

```bash
ollama pull ornith-1.5:35b
```

如果要直接测高于 Q4 的版本：

```bash
ollama pull hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:AD-Q5_K-Q4_K
```

质量优先再试：

```bash
ollama pull hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:AD-Q6_K
```

如下载失败或运行异常，再试：

```bash
ollama pull ornith:35b
```

### 2. Ollama 原生运行验收

```bash
ollama run ornith-1.5:35b "用一句中文说明你是否支持工具调用场景。"
```

检查点：

- 首 token 等待时间是否可接受。
- 输出是否稳定中文。
- 系统是否明显卡顿。
- 活动监视器中内存压力是否进入红色。

### 3. walle 配置

最小环境变量：

```bash
export LLM_MODEL=ollama/ornith-1.5:35b
export LLM_OLLAMA_FORMAT=openai
export LLM_OLLAMA_BASE_URL=http://localhost:11434/v1
```

高量化候选可直接把模型名换成完整 Ollama 名称：

```bash
export LLM_MODEL=ollama/hf.co/AtomicChat/Ornith-1.5-35B-A3B-GGUF:AD-Q5_K-Q4_K
```

如果用 CLI 临时切换：

```bash
./walle --model ollama/ornith-1.5:35b
```

### 4. walle 工具调用验收

在 `/Users/bytedance/Proj/5hWorkSpace` 里做，不要散落到 `/private/tmp`：

```bash
cd /Users/bytedance/Proj/5hWorkSpace
/Users/bytedance/Proj/walle/walle --model ollama/ornith-1.5:35b
```

建议人工输入：

```text
列出当前目录文件，然后读取 README.md 的前 20 行。
```

检查点：

- 是否会调用 `base.list_dir` / `base.read_file` 或等价工具。
- 工具参数 JSON 是否短、准、无多余字段。
- 工具结果返回后是否能继续总结。
- 是否出现假 tool call、重复 tool call、长时间无输出。

### 5. 压力边界检查

只做一个短任务即可，避免把验证变成大 benchmark：

```text
读取项目任务文件，选择一个 pending 任务，说明你会怎么做，先不要改文件。
```

观察：

- 长上下文下是否卡住。
- 是否过度推理导致响应太慢。
- 是否能遵守“先不要改文件”。

## 选型结论规则

| 观察结果 | 结论 |
| --- | --- |
| `ornith-1.5:35b` 能稳定调用工具，系统不卡 | 设为本机推荐模型 |
| `ornith-1.5:35b` 输出强但慢 | 作为复杂任务模型，日常保留 `ornith:9b` |
| `ornith-1.5:35b` 工具调用不稳 | 对照 `ornith:35b` |
| 35B 系列导致内存压力明显 | 停用 35B，本机继续用 `ornith:9b` |
| 后续确认真实存在 `25B A3B` 官方 tag | 重新单独评估 25B |

## 待核验

- `ornith-1.5:35b` 在当前 Ollama 版本 `0.32.1` 下的真实工具调用兼容性。
- walle 当前 OpenAI-compatible 调用链是否正确传递 Ollama 的 tool call schema。
- `ornith-1.5:35b` 默认 context 很大，实际运行时是否需要在 walle 侧限制上下文投影长度。
- 用户提到的 `25B A3B` 是否是口误、非官方量化包、或其他模型家族标签。

## 验收

完成本计划需要留下以下证据：

- `ollama list` 中存在目标模型。
- 一次 Ollama 原生中文运行输出。
- 一次 walle TUI 工具调用记录。
- 简短记录最终选择：推荐模型、fallback 模型、明显问题。
