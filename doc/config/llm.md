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

## 本地模型

本地模型推荐先走 Ollama 的 OpenAI-compatible `/v1` 接口，不需要为每个模型新增 Go provider：

```env
LLM_MODEL=ollama/ornith:9b
LLM_OLLAMA_FORMAT=openai
LLM_OLLAMA_BASE_URL=http://localhost:11434/v1
LLM_OLLAMA_API_KEY=dummy
```

`LLM_MODEL` 后半段会原样传给 Ollama，例如 `qwen3:14b`、`ornith:9b`、`ornith:35b` 或 HuggingFace GGUF 引用。Ollama library 中的 Ornith-1.0 面向 agentic coding，当前可用标签包括 `ornith:9b` 和 `ornith:35b`；日常 smoke test 优先用 `ornith:9b`，更重的代码任务再尝试 `ornith:35b`。

Ornith 属于较新的 Ollama library 模型；如果拉取时报 `requires a newer version of Ollama`，先升级 Ollama 客户端。

## Codex / ChatGPT 账户额度

Codex 是 OpenAI provider 的 ChatGPT OAuth 认证方式，不需要 `LLM_CODEX_API_KEY` 或 proxy 配置。在 TUI 输入 `/provider` 并选择 `openai`，完成浏览器登录后再从 `/model` 选择账号可用模型。

OAuth 凭据位于 `~/.5hAgent/auth/codex.json`（`0600`）。用户级默认模型位于 `~/.5hAgent/settings.json` 的 `default_model`；运行中的 runtime 模型选择位于 `~/.5hAgent/runtimes/<runtime-id>/state.json`。CLI `--model` 优先于 runtime 当前状态和用户默认值。

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
