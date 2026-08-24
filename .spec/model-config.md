# Model / Config / Logger Spec

## 职责

模型配置层把用户配置、prompt、provider adapter、Codex OAuth 和进程日志装配成 Runtime 可用的基础设施。它只提供模型实例、配置值、prompt 内容、模型目录和日志能力，业务执行由 Runtime/Agent 完成。

## 覆盖范围

| 路径 | 职责 |
| --- | --- |
| `internal/utils/utils.go` | Prompt、配置、用户状态、settings、路径、provider 发现。 |
| `internal/utils/tokenBudget.go` | token 预算和累计使用量。 |
| `internal/llm/client.go` | `LLMClient`、Claude/OpenAI/Codex model 创建、Ollama context window 探测。 |
| `internal/codex/auth.go` | ChatGPT OAuth PKCE、callback server、token 存储和刷新。 |
| `internal/codex/models.go` | 读取 ChatGPT 账号可用模型目录。 |
| `internal/codex/model.go` | Responses API stream adapter、tool call 名称映射。 |
| `internal/logger/logger.go` | 进程级文件日志、级别过滤、日志文件轮转。 |
| `internal/logger/color.go` | ANSI 颜色包装和日志文件颜色清理。 |

## 上游和下游

| 方向 | 模块 | 关系 |
| --- | --- | --- |
| 上游 | `internal/runtime` | 加载配置、prompt，创建模型，查询 provider/model。 |
| 上游 | `internal/agent` | 使用 token budget、debug request 和 logger。 |
| 上游 | `internal/tui` / daemon session | 通过 Runtime 间接发起 `/model`、`/provider`、login。 |
| 下游 | Eino `claude/openai` | 创建对应 `ToolCallingChatModel`。 |
| 下游 | ChatGPT Codex backend | OAuth、模型目录、Responses SSE。 |
| 下游 | 文件系统 | `~/.walle/.env/settings/auth/logs/prompt`。 |

## 入口接口

| 接口 | 输入 | 输出 | 行为 |
| --- | --- | --- | --- |
| `utils.GetConfigDir()` | 无 | `~/.walle` | 用户级配置根目录。 |
| `utils.GetProjectDataDir()` | 当前 cwd | `.walle` | 项目数据目录。 |
| `utils.LoadConfigWithOptions(opts)` | CLI 覆盖 | `AppConfig` | 合并配置来源并校验。 |
| `utils.ConfiguredProviders()` | 无 | provider 名列表 | 从 `.env` 和进程环境发现 provider。 |
| `utils.LoadSystemPromptBase(dir,base,provider,model)` | prompt 目录和模型 | prompt 文本 | 读取 base prompt 并叠加模型 prefix。 |
| `llm.NewClient(ctx,config)` | LLM config | `LLMClient` | 创建 Claude/OpenAI/Codex 模型。 |
| `LLMClient.ContextWindow(ctx)` | 当前配置 | int | Ollama `/api/ps` 探测上下文窗口，失败返回 0。 |
| `codex.DefaultStore()` | 无 | `Store` | 返回用户级 Codex 凭据仓库。 |
| `Store.StartLogin(ctx)` | context | login URL、done channel | 启动 OAuth PKCE 登录。 |
| `Store.Token(ctx)` | context | access token、account id | 读取或刷新凭据。 |
| `Store.Models(ctx)` | context | 模型名列表 | 查询 Codex 模型目录。 |
| `codex.NewModel(name)` | 模型名 | Eino model | 创建 Responses API adapter。 |
| `logger.InitLog()` / `CloseLog()` | 无 | log path / 无 | 打开或关闭进程级日志文件。 |

## 配置来源和优先级

模型选择使用 `provider/model` 形态。

| 配置项 | 优先级 | 说明 |
| --- | --- | --- |
| `opts.ModelRef` / `--model` | 1 | 完整覆盖 provider 和 model。 |
| `~/.walle/settings.json default_model` | 2 | 用户默认模型。 |
| `LLM_MODEL` | 3 | `.env` 或进程环境。 |

字段优先级：

1. CLI `--llm-format`、`--llm-model`。
2. 进程环境变量。
3. `~/.walle/.env`。
4. provider 默认值。

关键规则：

- `LLM_MODEL=provider/model` 是主模型引用。
- provider 配置块前缀是 `LLM_<PROVIDER>_`。
- `LLM_<PROVIDER>_FORMAT` 只表示接口协议：`claude`、`openai`、`codex`。
- `LLM_<PROVIDER>_BASE_URL`、`API_KEY`、`MAX_TOKENS`、`STREAM` 绑定当前 provider。
- `--llm-format` 和 `--llm-model` 只覆盖当前 provider 的接口协议或模型名。
- `openai/` 且 settings 中 `default_auth=chatgpt` 时走 `codex` 协议。
- `codex/` 模型引用默认走 `codex` 协议。

## Provider 默认值

| provider / format | 默认行为 |
| --- | --- |
| `claude` | 默认 base URL `https://api.anthropic.com`，默认模型 `claude-sonnet-4-6`。 |
| `openai` | 默认 base URL `https://api.openai.com/v1`，模型必须由引用或覆盖提供。 |
| `deepseek` | 未显式 format 时按 `openai`，base URL `https://api.deepseek.com`。 |
| `ollama` | 通过 OpenAI-compatible `/v1` 接入，通常配置 `LLM_OLLAMA_FORMAT=openai`。 |
| `codex` | base URL `https://chatgpt.com/backend-api/codex`，凭据来自 OAuth store。 |

`ConfiguredProviders` 会扫描 `LLM_MODEL` 和所有 `LLM_*_FORMAT`，仅做名称发现，不校验 API key 或 base URL 可用性。

## Prompt 规则

| 文件 | 用途 |
| --- | --- |
| `~/.walle/prompt/main.md` | 默认 base/process prompt。 |
| `~/.walle/prompt/tui.md` | TUI/interactive prompt base。 |
| `~/.walle/prompt/compress.md` | 上下文摘要压缩 prompt。 |
| `~/.walle/prompt/prefix.<provider>.<model-slug>.md` | 模型专用前缀。 |

规则：

- `Load(dir,name)` 读取 `<name>.md` 并 trim 首尾空白。
- `LoadOptional` 缺文件返回 `ok=false`。
- `LoadSystemPromptBase` base 为空时使用 `main`。
- 指定 base 缺失时回退 `main`。
- 模型 prefix 存在时拼到 base 前面，中间空一行。
- `ModelPromptSlug` 将 provider/model 小写化，非字母数字和 `-` 折叠为 `-`。

## LLM adapter 规则

| 协议 | 创建函数 | 规则 |
| --- | --- | --- |
| `claude` | `newClaudeModel` | 传入 API key、base URL、model、max tokens、可选 thinking。 |
| `openai` | `newOpenAIModel` | 使用 OpenAI-compatible Eino adapter。 |
| `codex` | `codex.NewModel` | 使用 ChatGPT OAuth 凭据调用 Responses API。 |

通用规则：

- `llm.NewClient` 会规范化 `Config.Provider`，`MaxTokens=0` 时补 `4096`。
- unsupported provider 返回 `unsupported LLM provider` 并列出支持值。
- `LLMClient.GetModel()` 返回 Eino `ToolCallingChatModel`。
- Runtime 负责把 `WithTools` 后的模型注入 Agent。

## Codex OAuth 和 Responses adapter

凭据路径：`~/.walle/auth/codex.json`，权限 `0600`。

OAuth：

- `StartLogin` 绑定 localhost callback 端口。
- 生成 PKCE verifier/challenge 和 state。
- 返回授权 URL，并尝试打开浏览器。
- callback 校验 state 和 code。
- token exchange 成功后提取 account ID 并原子写 credentials。
- 登录超时默认 10 分钟。

Token：

- `Store.Token` 在 access token 临近过期 5 分钟内刷新。
- `ForceRefresh` 用于 401 后强制刷新一次。
- 刷新时服务端未返回的字段沿用旧值。

Responses adapter：

- `WithTools` 返回模型副本，不修改原实例。
- 工具名中的 `.` 会映射为远端别名 `__`，例如 `base.read_file` -> `base__read_file`。
- alias 冲突会返回错误。
- system messages 汇总为 Responses `instructions`。
- user/assistant/tool messages 转为 Responses `input`。
- assistant tool call 转为 `function_call`，tool result 转为 `function_call_output`。
- `parallel_tool_calls=true` 是远端请求参数；本地 Agent 当前按单 worker 执行工具。
- `store=false`，避免远端保存会话。
- SSE 解析失败、非 2xx、401 刷新失败都要返回给 Agent。

## Token budget

- `TokenBudget` 由 Agent 创建，限制 `MaxTotalTokens`。
- 统计来自模型回调中的 usage。
- 内部用 mutex 保护累计值，允许 callback 与展示并发读取。
- 它只做预算和展示，不裁剪消息；裁剪由 Context 压缩负责。

## Logger 规则

- 日志目录：`~/.walle/logs/`。
- 文件名：`walle-<timestamp>.log`。
- 默认最多保留 30 个 `walle-*` 或旧 `walle-debug-*` 日志文件。
- `InitLog` 持锁创建新文件，关闭旧文件，切换进程级 writer。
- 日志只写文件，避免污染 TUI stdout/stderr。
- `colorStripWriter` 写文件前移除 ANSI 颜色码。
- `SetLevel` 调整进程级过滤级别。
- `CloseLog` 关闭当前文件并恢复 `io.Discard`。

## 状态边界

- 用户配置目录：`~/.walle`。
- 项目数据目录：当前工作目录下 `.walle`。
- settings/env 是配置来源；运行进程状态由 supervisor socket 提供。
- Codex 凭据只在用户级 auth 目录。
- logger 是进程级全局状态，业务状态不要放入 logger。
- utils 不持有长生命周期缓存。

## 错误处理和安全

- 缺失 `LLM_MODEL` 或格式错误要返回明确错误。
- 缺 provider format 要返回 `LLM_<PROVIDER>_FORMAT is required`。
- OAuth state 不匹配要拒绝 callback。
- token、API key、account id 不得写入日志、debug request、session 或 report。
- Codex HTTP 非 2xx 要读取有限长度错误正文，避免无限读。
- Ollama context window 探测失败返回 0，不阻断主流程。
- logger 初始化失败阻断 Runtime 创建，避免缺失诊断链路。

## 禁止

- 在配置层调用 Agent、工具、TUI 或 daemon。
- 为每个本地模型新增专用 provider；优先复用 provider 配置块。
- 把 Codex 凭据写入项目 `.walle/`。
- 把 API key、OAuth token、account id 输出到可见日志或报告。
- 在 logger 中引入业务包依赖。
- 把 prompt 加载失败静默吞掉后继续启动。

## 修改检查

- 改配置优先级：同步 README、`doc/config/llm.md`、Runtime `SwitchModel`。
- 改 provider 默认值：检查 `LoadConfigWithOptions`、`ConfiguredProviders`、`ProviderModels`。
- 改 prompt 加载：检查 `prompt/main.md`、`prompt/tui.md`、`prompt/compress.md`、模型 prefix。
- 改 Codex OAuth：检查 callback 端口、state 校验、原子写凭据、刷新失败路径。
- 改 Responses adapter：检查 tool alias 成对映射、SSE 事件、tool call/result 重放。
- 改 logger：检查 TUI stdout/stderr、日志轮转、ANSI 清理和 `CloseLog`。
