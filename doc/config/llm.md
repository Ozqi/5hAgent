# LLM

## 职责

`internal/llm` 把当前 provider 配置转换成 Eino `model.ToolCallingChatModel`。配置层只负责读取 provider、model、接口格式和凭据，不创建 Agent、不处理 TUI。

## 先读 `LLM_MODEL`

`LLM_MODEL` 是入口：

```env
LLM_MODEL=<provider>/<model>
```

- `/` 前面的 `<provider>` 只用于选择配置块。
- `/` 后面的 `<model>` 原样传给上游，可以继续包含 `/`。
- provider 名会映射到 `LLM_<PROVIDER>_*`，例如 `mygateway/<model>` 读取 `LLM_MYGATEWAY_*`。

最小配置长这样：

```env
LLM_MODEL=mygateway/<model>

LLM_MYGATEWAY_FORMAT=openai
LLM_MYGATEWAY_BASE_URL=https://example.com/v1
LLM_MYGATEWAY_API_KEY=sk-...
LLM_MYGATEWAY_MAX_TOKENS=4096
LLM_MYGATEWAY_THINKING_BUDGET_TOKENS=0
LLM_MYGATEWAY_STREAM=true
```

如果只是临时切换，不需要改 `.env`：

```bash
walle --model <provider>/<model>
```

## Provider 块

每个 provider 都是一组同名前缀变量：

| 字段 | 说明 |
| --- | --- |
| `LLM_<PROVIDER>_FORMAT` | 接口格式：`claude`、`openai` 或 `codex`。 |
| `LLM_<PROVIDER>_BASE_URL` | 上游接口地址。OpenAI-compatible 网关通常是 `/v1` 结尾。 |
| `LLM_<PROVIDER>_API_KEY` | 上游密钥。本地服务可用 `dummy`。 |
| `LLM_<PROVIDER>_MAX_TOKENS` | 单次生成上限。 |
| `LLM_<PROVIDER>_THINKING_BUDGET_TOKENS` | Claude thinking 预算；OpenAI-compatible 一般填 `0`。 |
| `LLM_<PROVIDER>_STREAM` | 是否启用流式输出，默认建议 `true`。 |

`FORMAT` 是接口协议，不是供应商名称。`mygateway`、`local` 这类名字只是 provider 配置块名称。

## OpenAI-compatible 网关

OpenAI-compatible 网关只需要新增一个 provider 块。示例：

```env
LLM_MODEL=mygateway/<model>

LLM_MYGATEWAY_FORMAT=openai
LLM_MYGATEWAY_BASE_URL=https://example.com/v1
LLM_MYGATEWAY_API_KEY=sk-...
LLM_MYGATEWAY_MAX_TOKENS=4096
LLM_MYGATEWAY_THINKING_BUDGET_TOKENS=0
LLM_MYGATEWAY_STREAM=true
```

把 `mygateway` 换成你的 provider 名。provider 名可以按服务名起，例如 `acme`；对应环境变量前缀就是 `LLM_ACME_*`。

如果平台文档提供的地址已经包含 `/v1`，照文档填写；如果只给域名，通常需要补成 OpenAI-compatible 的 `/v1` 地址。

## 本地 OpenAI-compatible 服务

本地模型服务也走同一套 provider 机制，不需要为某个具体模型新增 Go provider：

```env
LLM_MODEL=local/<model>

LLM_LOCAL_FORMAT=openai
LLM_LOCAL_BASE_URL=http://localhost:11434/v1
LLM_LOCAL_API_KEY=dummy
LLM_LOCAL_MAX_TOKENS=4096
LLM_LOCAL_THINKING_BUDGET_TOKENS=0
LLM_LOCAL_STREAM=true
```

`LLM_MODEL` 后半段会原样传给本地服务。具体模型名、拉取命令和服务启动方式以本地服务自己的文档为准。

## Claude / Anthropic Messages API

```env
LLM_MODEL=claude/<model>

LLM_CLAUDE_FORMAT=claude
LLM_CLAUDE_BASE_URL=https://api.anthropic.com
LLM_CLAUDE_API_KEY=sk-ant-...
LLM_CLAUDE_MAX_TOKENS=4096
LLM_CLAUDE_THINKING_BUDGET_TOKENS=0
LLM_CLAUDE_STREAM=true
```

## Codex / ChatGPT 账户额度

Codex 是 OpenAI provider 的 ChatGPT OAuth 认证方式，不需要 `LLM_CODEX_API_KEY` 或 proxy 配置。在 TUI 输入 `/provider` 并选择 `openai`，完成浏览器登录后再从 `/model` 选择账号可用模型。

OAuth 凭据位于 `~/.walle/auth/codex.json`（`0600`）。用户级默认模型位于 `~/.walle/settings.json` 的 `default_model`。CLI `--model` 只覆盖本次 Runtime，优先于用户默认值；`SwitchModel` 只更新当前 Runtime 内存。

## 文件

| 文件 | 作用 |
| --- | --- |
| [client.go](../../internal/llm/client.go) | 按 format 创建 Claude/OpenAI-compatible/Codex model。 |
| [utils.go](../../internal/utils/utils.go) | 解析 `.env`、CLI override、模型引用。 |
| [runtime.go](../../internal/runtime/runtime.go) | 创建 client 并绑定工具。 |

## CLI override

| 参数 | 作用 |
| --- | --- |
| `--model provider/model` | 临时切换完整模型引用。 |
| `--llm-format` | 临时覆盖接口格式。 |
| `--llm-model` | 临时覆盖模型名。 |

这些参数不改写 `~/.walle/.env`。
