# Codex 账户额度接入

问题：能不能让 5hAgent 接入 Codex 账户，走 Codex/ChatGPT 账户侧的额度，而不是单独配置 OpenAI API key 额度。

## 设计问题

- 当前 5hAgent 的 LLM 配置是 `LLM_MODEL=supplier/model`，再通过 `LLM_<SUPPLIER>_FORMAT`、`BASE_URL`、`API_KEY` 接到 OpenAI-compatible 或 Claude-compatible API。
- Codex 账户额度可能不是普通 OpenAI API key 额度；不能默认认为填 `LLM_OPENAI_API_KEY` 就是在走 Codex 账户。
- 如果 Codex CLI/账户有本地登录态、token broker 或代理服务，需要先确认它是否提供稳定的本地 API、认证方式和使用条款。

## 待确认
- [x] Codex 账户额度是否有公开、允许复用的 API 通道，还是只允许官方 Codex 客户端使用。
- [x] 本机 Codex CLI 登录态是否能被第三方程序安全读取或通过本地服务转发。
- [x] 如果只能通过官方 CLI 调用，是否应该做成独立 provider/proxy，而不是塞进 `internal/llm`。
- [x] 需要明确 token/credential 存放边界：不能把账户 token 写入项目 `.5hagent/` 或 session 文件。

- 能不能做成就是非命令模式，就是做成聊天，TUI聊天也这个slash model这种。就比如说你敲了 /provider 选择codex登录这样子, 然后你用杠model会显示这个Provider下的所有的模型。

## 当前实现结论

- [x] 不依赖本机 `codex` 程序；5hAgent 内置 ChatGPT OAuth PKCE 登录。
- [x] TUI 用 `/provider` 选择 `openai`，该 provider 的认证方式是 ChatGPT OAuth；Codex 是套餐/后端能力，不依赖本机 `codex` 程序。
- [x] 登录后 `/model` 展示该账号通过 Codex models endpoint 返回的模型。
- [x] provider/model 选择保存到 `~/.5hAgent/state.json`，OAuth 凭据保存到 `~/.5hAgent/auth/codex.json`。

## 完成记录

- Codex OAuth provider 设计文档：`32c806f`
- 内置 OpenAI ChatGPT OAuth provider 与 Codex Responses 最小闭环：`待回填`
