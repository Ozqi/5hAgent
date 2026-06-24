# Prompt 目录

本目录存放运行时直接读取的 Markdown prompt 文件。安装后会复制到 `~/.5hAgent/prompt/`，运行时以用户目录为准。

## 约定

- 代码通过 `utils.Load(dir, name)` 读取 `<dir>/<name>.md`。
- 文件内容会被直接使用，不解析 frontmatter，不做变量替换。
- `main.md` 是全局 Agent system prompt，必须存在。
- 可选模型前缀使用 `prefix.<provider>.<model-slug>.md`，存在时叠加到 `main.md` 前面。

## 模型前缀命名

`model-slug` 由当前模型名生成：转小写，字母数字和 `-` 保留，其他字符折叠为 `-`。

示例：

```text
qwen3:14b
=> prefix.openai.qwen3-14b.md

hf.co/bartowski/Qwen_Qwen3.6-27B-GGUF:Q3_K_M
=> prefix.openai.hf-co-bartowski-qwen-qwen3-6-27b-gguf-q3-k-m.md
```

## 当前文件

- `main.md`: 主 Agent system prompt
- `compress.md`: 上下文压缩 prompt
- `plan.md`: 规划相关 prompt
- `worker.md`: worker 相关 prompt

## 示例

```go
content, err := utils.Load("prompt", "main")
```

会读取：

```text
prompt/main.md
```
