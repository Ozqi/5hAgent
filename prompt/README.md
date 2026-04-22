# Prompt 管理系统

本目录用于管理 5hAgent 项目中的所有提示词（Prompts）。

## 目录结构

```
prompt/
├── README.md           # 本文档
├── system/             # 系统提示词
│   ├── main_agent.md           # 主 Agent 系统提示词
│   ├── sub_agent_template.md   # Sub-Agent 模板
│   ├── code_reviewer.md        # 代码审查 Agent
│   └── task_planner.md         # 任务规划 Agent
├── user/               # 用户提示词模板（可选）
└── examples/           # 示例提示词（可选）
```

## 提示词文件格式

每个提示词文件使用 Markdown 格式，包含 YAML frontmatter 和内容两部分：

```markdown
---
name: prompt_name              # 提示词名称（必需，用于代码中引用）
description: 提示词描述        # 描述（必需）
version: "1.0"                 # 版本号（可选）
type: system                   # 类型: system/user/assistant（必需）
variables:                     # 变量定义（可选）
  var1: "默认值1"
  var2: "默认值2"
---

这里是提示词的实际内容。

可以使用 {{var1}} 和 {{var2}} 来引用变量。
```

## 使用方法

### 1. 在代码中加载提示词

```go
import "github.com/lzq/5hAgent/internal/prompt"

// 创建加载器
loader := prompt.NewLoader("prompt")

// 加载所有提示词
if err := loader.Load(); err != nil {
    log.Fatal(err)
}

// 获取提示词内容
content, err := loader.GetContent("main_agent_system")
if err != nil {
    log.Fatal(err)
}
```

### 2. 使用变量替换

```go
// 获取带变量的提示词
vars := map[string]string{
    "task_description": "实现用户认证功能",
    "constraints": "必须使用 JWT，不能使用明文密码",
}

content, err := loader.GetContentWithVars("sub_agent_template", vars)
if err != nil {
    log.Fatal(err)
}
```

### 3. 列出所有提示词

```go
prompts := loader.List()
for _, p := range prompts {
    fmt.Printf("Name: %s, Type: %s, Version: %s\n", 
        p.Name, p.Type, p.Version)
}
```

## 提示词类型

- **system**: 系统提示词，定义 Agent 的角色和行为规则
- **user**: 用户提示词模板，用于生成用户消息
- **assistant**: 助手提示词模板，用于生成助手响应示例

## 变量系统

提示词支持变量替换，使用 `{{variable_name}}` 语法：

- 在 frontmatter 中定义变量及其默认值
- 在内容中使用 `{{variable_name}}` 引用变量
- 在代码中通过 `GetContentWithVars()` 传入实际值

## 最佳实践

1. **命名规范**: 使用小写字母和下划线，如 `main_agent_system`
2. **版本管理**: 重大修改时更新版本号
3. **描述清晰**: 在 description 中说明提示词的用途和适用场景
4. **模块化**: 将不同功能的提示词分开管理
5. **变量使用**: 对于需要动态内容的部分使用变量
6. **文档同步**: 修改提示词时同步更新相关文档

## 注意事项

- 提示词文件必须使用 UTF-8 编码
- frontmatter 必须以 `---` 开始和结束
- `name` 字段必须唯一
- 修改提示词后需要重启应用或调用 `Reload()` 方法
