# lsp

参考opencode实现语法检测功能。

## 确定结论

- 语法检测应先做成“可选验证能力”，不要进入每次工具编辑后的强制路径。
- 最小实现可以先支持 Go：保存后运行 `gofmt`/`go test` 或 `gopls` diagnostics；多语言 LSP 后置。
- 如果接入 LSP，应该通过项目配置启用，并把诊断作为工具结果或 hook 产物返回给 Agent。
- 本条依赖 runtime hook 或工具失败统计时机，当前不作为本次实现点。
