# LLM

## 职责

`internal/llm` 把当前 provider 配置转换成 Eino `model.ToolCallingChatModel`。

## 配置主线

```env
LLM_MODEL=provider/model-name
LLM_<PROVIDER>_FORMAT=claude|openai
LLM_<PROVIDER>_BASE_URL=...
LLM_<PROVIDER>_API_KEY=...
```

`provider` 只用于查配置块；`model-name` 原样传给上游。

## 文件

| 文件 | 作用 |
| --- | --- |
| [client.go](../../internal/llm/client.go) | 按 format 创建 Claude/OpenAI-compatible model |
| [utils.go](../../internal/utils/utils.go) | 解析 `.env`、CLI override、模型引用 |
| [runtime.go](../../internal/runtime/runtime.go) | 创建 client 并绑定工具 |

## Format

| format | 含义 |
| --- | --- |
| `claude` | Anthropic Messages 类接口 |
| `openai` | OpenAI-compatible chat completions |

`claude` / `openai` 是接口格式，不是供应商。

## CLI override

| 参数 | 作用 |
| --- | --- |
| `--model provider/model` | 临时切换完整模型引用 |
| `--llm-format` | 临时覆盖接口格式 |
| `--llm-model` | 临时覆盖模型名 |

这些参数不改写 `~/.5hAgent/.env`。
