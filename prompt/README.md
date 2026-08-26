# Prompt 目录

本目录存放运行时直接读取的 Markdown prompt 文件。安装脚本只把基础运行 prompt 复制到 `~/.walle/prompt/`，模型专用 `prefix.*.md` 默认留在仓库作为可选参考；运行时以用户目录为准。

## 约定

- 代码通过 `utils.Load(dir, name)` 读取 `<dir>/<name>.md`。
- 文件内容会被直接使用，不解析 frontmatter，不做变量替换。
- `main.md` 是 headless/daemon 默认 system prompt，必须存在。
- `tui.md` 是 TUI 交互入口 system prompt；旧安装缺失时回退到 `main.md`。
- 可选模型前缀使用 `prefix.<provider>.<model-slug>.md`，存在时叠加到 `main.md` 前面。

## 模型前缀命名

`model-slug` 由当前模型名生成：转小写，字母数字和 `-` 保留，其他字符折叠为 `-`。

示例：

```text
provider=mygateway, model=family/model:tag
=> prefix.mygateway.family-model-tag.md
```

## 当前文件

- `main.md`: headless/daemon system prompt
- `tui.md`: TUI 交互 system prompt
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
