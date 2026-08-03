# Codex 账户额度接入

问题：能不能让 5hAgent 接入 Codex 账户，走 Codex/ChatGPT 账户侧的额度，而不是单独配置 OpenAI API key 额度。

## 设计问题

- 当前 5hAgent 的 LLM 配置是 `LLM_MODEL=supplier/model`，再通过 `LLM_<SUPPLIER>_FORMAT`、`BASE_URL`、`API_KEY` 接到 OpenAI-compatible 或 Claude-compatible API。
- Codex 账户额度可能不是普通 OpenAI API key 额度；不能默认认为填 `LLM_OPENAI_API_KEY` 就是在走 Codex 账户。
- 如果 Codex CLI/账户有本地登录态、token broker 或代理服务，需要先确认它是否提供稳定的本地 API、认证方式和使用条款。
- 设计上应优先走“外部 adapter/proxy”接入，把它暴露成 OpenAI-compatible endpoint；不要为了某个账户体系直接改核心 LLM 抽象。

## 待确认

- [x] Codex 账户额度是否有公开、允许复用的 API 通道，还是只允许官方 Codex 客户端使用。
- [x] 本机 Codex CLI 登录态是否能被第三方程序安全读取或通过本地服务转发。
- [x] 如果只能通过官方 CLI 调用，是否应该做成独立 provider/proxy，而不是塞进 `internal/llm`。
- [x] 需要明确 token/credential 存放边界：不能把账户 token 写入项目 `.5hagent/` 或 session 文件。

## 最小可接受方向

- 如果存在合法稳定的 OpenAI-compatible 转发层，5hAgent 只新增一个 `supplier` 配置示例，例如 `LLM_MODEL=codex/<model>`。
- 如果不存在公开 API，就只记录为不可直接接入；不要用抓包、复用私有 token 或模拟官方客户端的方式绕过账户边界。

## 结论

- 5hAgent 不直接读取 Codex CLI / ChatGPT token，也不把 Codex 账户体系塞进 `internal/llm`。
- 仅支持通过可信本地 OpenAI-compatible proxy 作为 `codex` provider 接入，例如 `LLM_MODEL=codex/gpt-5.1` + `LLM_CODEX_FORMAT=openai`。
- 新增 `5hagent codex` 诊断命令，只检查 Codex CLI 登录态和 `LLM_CODEX_*` 配置完整性，不读取或打印 token。

## 完成记录

- Codex provider 配置示例与安全诊断命令：`待回填`
