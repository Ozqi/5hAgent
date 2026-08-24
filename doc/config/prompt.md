# Prompt

## 职责

Prompt 文件安装到 `~/.walle/prompt/`，运行时以用户目录为准。

## 文件

| 文件 | 用途 |
| --- | --- |
| `main.md` | Runtime 默认 system prompt |
| `tui.md` | TUI system prompt |
| `prefix.<provider>.<model>.md` | 可选模型前缀 |
| `compress.md` | LLM 压缩 prompt |
| `plan.md` | 规划 prompt，占位 |
| `worker.md` | worker prompt，占位 |

## 加载

| 函数 | 行为 |
| --- | --- |
| `utils.Load` | 严格读取 `<name>.md` |
| `LoadSystemPromptBase` | 加载指定 base；TUI 使用 `tui.md`，缺失回退 `main.md` |
| `ModelPrefixPromptName` | 生成 prefix 文件名 |

## 入口差异

- 默认 CLI 启动的 daemon 设置 `PromptBase="tui"`，供 attached TUI 使用。
- 其他 Runtime 调用未指定时使用 `main.md`。
- `/model` 切换会保留当前 Runtime 的 `PromptBase`。

## 约束

已有 session 中已经注入的 system message 不会自动替换。prompt 改动只影响新建会话。
