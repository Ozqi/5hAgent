# Prompt 目录

本目录存放运行时直接读取的 Markdown prompt 文件。

## 约定

- 代码通过 `utils.Load(dir, name)` 读取 `<dir>/<name>.md`
- 当前主流程默认从 `prompt/` 目录读取
- 文件内容会被直接使用，不解析 frontmatter

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
