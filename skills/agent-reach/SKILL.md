---
name: agent-reach
description: 用户要求联网搜索、调研、查找资料、读取 URL，或提到 Twitter/X、Reddit、Facebook、Instagram、YouTube、GitHub、Bilibili、小红书、小宇宙、LinkedIn、V2EX、雪球、RSS 时使用。只负责获取互联网内容，不负责发帖、评论、点赞或代替用户登录。
---

# Agent Reach

使用 Agent Reach 选择当前可用的互联网后端，不猜测平台命令。

## 固定流程

1. 多后端或依赖登录态的平台先检查：

```bash
agent-reach doctor --json
```

2. 根据 `active_backend` 选择命令，并告诉用户正在使用的平台和后端。
3. 只做读取和研究；登录、发帖、评论、点赞等写操作不在本 Skill 范围内。
4. 宽泛调研可并行组合 Web 搜索、社区讨论和中文内容，再统一归纳。
5. 临时文件放 `/tmp/`，持久数据放 `~/.agent-reach/`，不要污染项目目录。

## 常用入口

```bash
# Web 搜索与代码上下文
mcporter call 'exa.web_search_exa(query: "query", numResults: 5)'
mcporter call 'exa.get_code_context_exa(query: "question", tokensNum: 3000)'

# 网页阅读
curl -s "https://r.jina.ai/URL"

# GitHub
gh search repos "query" --sort stars --limit 10
gh search code "query" --language go

# YouTube
yt-dlp --write-sub --write-auto-sub --skip-download -o "/tmp/%(id)s" "URL"

# Bilibili
bili search "query" --type video -n 5

# V2EX
curl -s "https://www.v2ex.com/api/topics/hot.json" -H "User-Agent: agent-reach/1.0"
```

## 登录态平台

Twitter/X、Reddit、小红书、Facebook、Instagram 必须先依据 `doctor` 的结果选择后端。只使用用户已明确建立的登录态或手工提供的凭据，不读取浏览器 Cookie，不自动登录。

常见后端命令：

```bash
opencli twitter search "query" -f yaml
opencli reddit search "query" -f yaml
opencli xiaohongshu search "query" -f yaml
opencli facebook search "query" -f yaml
opencli instagram search "query" -f yaml
```

如果 `agent-reach` 或对应后端不可用，报告 `doctor` 的具体结果和缺失依赖，不自行安装或绕过平台限制。
