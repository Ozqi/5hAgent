# Prompt - 提示词加载

```text
prompt/*.md
  -> internal/utils/utils.go Load()
  -> cmd/5hagent/main.go / internal/context/ctx.go
```

## 位置

- `internal/utils/utils.go`
- `internal/utils/utils_test.go`
- `prompt/*.md`
- `cmd/5hagent/main.go`
- `internal/context/ctx.go`

## 概述

当前 prompt 系统是一个非常轻的文件读取约定，没有单独的 loader、缓存和 frontmatter 解析。

调用方直接使用：

```go
content, err := utils.Load("prompt", "main")
```

它会读取 `prompt/main.md`，返回去掉首尾空白后的内容。

## 当前调用点

- `cmd/5hagent/main.go` 用 `utils.Load("prompt", "main")` 加载主 system prompt
- `internal/context/ctx.go` 用 `utils.Load(promptDir, "compress")` 加载上下文压缩 prompt
- `internal/commands/compress.go` 通过 `internal/context/ctx.go` 的手动压缩入口复用同一个 `prompt/compress.md`

## 文件约定

- prompt 文件放在 `prompt/` 目录下
- 文件名就是调用名加 `.md`
- 当前实际使用的是：
  - `prompt/main.md`
  - `prompt/compress.md`
  - `prompt/plan.md`
  - `prompt/worker.md`

例如：

- `utils.Load("prompt", "main")` -> `prompt/main.md`
- `utils.Load("prompt", "compress")` -> `prompt/compress.md`

## 设计边界

- 不递归扫描目录
- 不解析 frontmatter
- 不做变量替换
- 不做缓存
- 找不到文件时直接返回错误

这个设计刻意保持简单，适合当前仓库里“少量固定 prompt 文件”的使用方式。

## 相关代码

- [utils.go](../internal/utils/utils.go)
- [utils_test.go](../internal/utils/utils_test.go)
- [main.go](../cmd/5hagent/main.go)
- [ctx.go](../internal/context/ctx.go)
