# Skill 注入机制实现文档

## 概述

Skill 注入机制允许动态加载和启用预定义的技能提示词，增强 Agent 在特定场景下的能力。技能作为独立的 System 消息插入到对话上下文中，而不是混入主 System Prompt。

**标准格式**: 使用 Markdown 文件 + YAML frontmatter（遵循 Claude Code 规范）

## 架构

```
.5hagent/skills/          # 技能定义目录
├── code-review/
│   └── SKILL.md            # 代码审查技能
├── debug-helper/
│   └── SKILL.md            # 调试助手技能
└── superpower/
    └── SKILL.md            # 生产力提升技能

internal/skill/
└── skill.go                # 技能管理器（解析 YAML frontmatter）

internal/agent/
├── agent.go                # Agent 集成技能注入
└── skill.go                # 技能注入和命令处理
```

## Skill 文件格式

### 标准结构

```markdown
---
name: skill-name
description: Use when user asks to "trigger phrase", "another phrase", or mentions keyword. Brief description of what this skill provides.
version: 1.0.0
tools: Read, Grep, Bash
---

# Skill Title

Skill content in Markdown format...

## Section 1
...

## Section 2
...
```

### Frontmatter 字段

- **name** (必需): 技能标识符，用于 `/skill enable <name>`
- **description** (必需): 触发条件描述，告诉 LLM 何时使用此技能
- **version** (可选): 语义化版本号
- **tools** (可选): 技能可能用到的工具列表

### Description 编写规范

Description 是最关键的字段，决定 LLM 何时自动激活技能。

**好的 description 模式**:
```yaml
description: Use when user asks to "specific phrase", "another phrase", mentions "keyword", or discusses topic-area.
```

**包含内容**:
- 用户可能说的具体短语（用引号标注）
- 相关关键词
- 适用的主题领域

## 核心组件

### 1. Skill 结构

```go
type Skill struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Version     string `yaml:"version,omitempty"`
    Tools       string `yaml:"tools,omitempty"`
    Content     string `yaml:"-"` // Markdown 内容
    Enabled     bool   `yaml:"-"` // 运行时状态
}
```

### 2. Manager 技能管理器

**功能**:
- `LoadSkills()`: 从 `.5hagent/skills/*/SKILL.md` 加载技能定义
- `EnableSkill(name)`: 启用指定技能
- `DisableSkill(name)`: 禁用指定技能
- `ListSkills()`: 列出所有技能

**加载逻辑**:
1. 遍历 `.5hagent/skills/` 下的所有子目录
2. 查找每个子目录中的 `SKILL.md` 文件
3. 解析 YAML frontmatter 和 Markdown 内容
4. 默认状态为禁用

### 3. Agent 集成

**注入时机**: 在首次对话时，System Prompt 之后、User Message 之前

**注入方式**: 每个启用的技能作为独立的 System 消息插入

```go
// 消息顺序示例
[
  {role: "system", content: "You are a helpful AI assistant."},
  {role: "system", content: "# Skill: superpower\n\n[skill content]"},
  {role: "user", content: "用户输入"}
]
```

**优势**:
- 职责分离：System Prompt 和 Skill 独立管理
- 动态性：可以在不修改 System Prompt 的情况下启用/禁用技能
- 可追踪：每个技能作为独立消息，便于调试和日志记录

## 使用方式

### 命令行交互

```bash
# 列出所有技能
/skill list

# 启用技能
/skill enable superpower

# 禁用技能
/skill disable superpower
```

### 输出示例

```
Available Skills:
  - code-review [disabled]: Use when user asks to "review code"...
  - debug-helper [disabled]: Use when user asks to "debug"...
  - superpower [enabled]: Use when user asks to "boost productivity"...
```

## 内置技能

### code-review
代码审查技能，提供质量、安全性、性能审查清单。

**触发条件**: "review code", "check quality", "audit security", "improve performance"

### debug-helper
系统化调试方法论，提供五阶段调试流程。

**触发条件**: "debug", "troubleshoot", "find bug", "fix error"

### superpower
开发者生产力提升技能，包含命令行、Git、编辑器等高级技巧。

**触发条件**: "boost productivity", "work faster", "optimize workflow", "supercharge development"

## 扩展

### 创建新技能

1. 在 `.5hagent/skills/` 下创建新目录
2. 创建 `SKILL.md` 文件
3. 编写 YAML frontmatter 和 Markdown 内容
4. 重启 Agent 自动加载

**示例**:
```bash
mkdir -p .5hagent/skills/my-skill
cat > .5hagent/skills/my-skill/SKILL.md << 'EOF'
---
name: my-skill
description: Use when user asks to "do something specific".
---

# My Skill

Skill content here...
EOF
```

### 添加支持文件

技能可以包含额外的参考文件：

```
skills/
└── my-skill/
    ├── SKILL.md          # 主技能定义
    ├── README.md         # 额外文档
    ├── references/       # 参考资料
    │   └── patterns.md
    ├── examples/         # 示例文件
    │   └── sample.md
    └── scripts/          # 辅助脚本
        └── helper.sh
```

## 实现细节

### loadSkillFile() 函数

位于 `internal/skill/skill.go`，负责解析 SKILL.md 文件：

```go
func (m *Manager) loadSkillFile(path string) (*Skill, error) {
    data, err := os.ReadFile(path)
    // ...
    
    // 分离 frontmatter 和内容
    parts := strings.SplitN(content[4:], "\n---\n", 2)
    
    // 解析 YAML frontmatter
    var skill Skill
    yaml.Unmarshal([]byte(parts[0]), &skill)
    
    // 保存 Markdown 内容
    skill.Content = strings.TrimSpace(parts[1])
    skill.Enabled = false
    
    return &skill, nil
}
```

### injectSkills() 函数

位于 `internal/agent/skill.go`，负责将启用的技能转换为 System 消息：

```go
func (a *Agent) injectSkills(messageCtx *agentctx.Context) error {
    skills := a.skillManager.ListSkills()
    for _, skill := range skills {
        if skill.Enabled {
            skillMsg := &schema.Message{
                Role:    schema.System,
                Content: fmt.Sprintf("# Skill: %s\n\n%s", 
                    skill.Name, skill.Content),
            }
            a.ctxManager.AddMessage(messageCtx, skillMsg)
        }
    }
    return nil
}
```

### 注入流程

1. 检查是否为首次对话（消息列表为空）
2. 添加主 System Prompt
3. 调用 `injectSkills()` 添加启用的技能
4. 添加用户消息
5. 开始 ReAct 循环

## 最佳实践

### Skill 设计原则

1. **单一职责**: 每个 skill 专注一个领域
2. **清晰触发**: description 明确指出何时激活
3. **结构化内容**: 使用 Markdown 标题组织信息
4. **可操作指导**: 提供具体步骤，不只是理论
5. **包含示例**: 实际例子帮助理解

### Description 编写技巧

- 使用 "Use when user asks to..." 开头
- 列举具体的触发短语（用引号）
- 包含相关关键词
- 描述适用场景
- 避免与其他 skill 重叠

### 内容组织

- 使用清晰的标题层级
- 提供检查清单或步骤列表
- 包含代码示例（如适用）
- 添加常见问题或注意事项
- 保持简洁，避免冗长

## 与 Claude Code 的兼容性

本实现遵循 Claude Code 的 skill 规范：
- ✅ YAML frontmatter 格式
- ✅ SKILL.md 文件命名
- ✅ name 和 description 字段
- ✅ 目录结构（skills/skill-name/SKILL.md）
- ⚠️ 部分字段未实现（user-invocable, disable-model-invocation, context）

未来可扩展支持更多 Claude Code 特性。
