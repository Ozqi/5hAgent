# Markdown 读取工具改进

`base.read_md` 已支持标题列表、section 读取、替换和删除。

## 待实现

- [ ] 增加跨文件复制 section 的最小接口。
- [ ] 增加剪切 section 的目标路径、覆盖策略和失败回滚语义。
- [ ] 复用现有 section 读取与写入能力，不新增 Markdown 解析框架。

动态持久工具归 [10_动态终端工具.md](10_动态终端工具.md) 统一设计。
