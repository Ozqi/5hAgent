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

5hAgent 不直接读取 `~/.codex/auth.json`，也不复用 Codex CLI 的私有 token。若要使用 Codex/ChatGPT 账户侧额度，推荐在本机启动一个可信的 OpenAI-compatible Codex proxy，然后把它作为普通 provider 接入：

```env
LLM_MODEL=codex/gpt-5.1
LLM_CODEX_FORMAT=openai
LLM_CODEX_BASE_URL=http://127.0.0.1:8787/v1
LLM_CODEX_API_KEY=codex-proxy
LLM_CODEX_STREAM=true
```

诊断当前机器是否具备接入条件：

```bash
5hagent codex
```

该命令只检查 Codex CLI 是否存在、`codex login status` 是否成功，以及 `LLM_CODEX_*` 是否配置完整；不会读取或打印任何账户 token。若没有公开、稳定、符合使用条款的 proxy/API 通道，应视为不可直接接入，不要通过抓包、复制私有 token 或模拟官方客户端绕过账户边界。

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
