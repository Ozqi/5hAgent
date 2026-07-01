# Stage 3 导读

这一阶段对应 `learn/stage-3-skill-prompt`。

前两个阶段主要解决的是“先跑起来”和“跑得更顺”，而这一阶段开始回答另一个问题：怎样让 Agent 的行为更稳定、更可复用，而不是每次都完全依赖一段硬编码 prompt 或一次临场发挥。

所以 Stage 3 的重点，不是单纯再加几个工具，而是开始整理“能力是怎么被组织起来的”。

## 这一阶段新增了什么

- Prompt 文件化管理，不再只靠代码里硬编码的提示词
- Skill 机制，让一组特定场景下的行为约束可以被单独启用
- 更清晰的任务工具与命令入口
- 更完整的工具集，开始接近 Coding Agent 的常见工作流

可以先用下面这些浅解释来理解新概念：

- `Prompt`：给模型的指令文本，告诉它应该怎么做事
- `Prompt 文件化`：把这些指令从代码里拿出来，变成独立的 Markdown 文件，方便维护和替换
- `Skill`：面向特定场景的一组附加规则，可以理解成“按需加载的小型行为包”
- `frontmatter`：Markdown 文件开头的元数据区块，用来写名字、描述等信息
- `Task CRUD`：对任务进行创建、读取、更新、删除

## 推荐阅读顺序

建议这一阶段按“为什么需要这些机制”来读，而不是按文件多少来读：

1. [doc/phase3_summary.md](./phase3_summary.md)
   先看这一阶段想解决什么问题，别急着埋进实现细节。
2. [doc/phase2_summary.md](./phase2_summary.md)
   快速回看上一阶段，感受为什么项目会自然发展到 prompt/skill 这一层。
3. [doc/agent.md](./agent.md)
   看 Skill 是怎么进入主循环的。
4. [doc/prompt/README.md](./prompt/README.md)
   看 Prompt 是怎么从代码迁移到文件的。
5. [doc/tools.md](./tools.md) 和 [doc/context.md](./context.md)
   作为补充，理解它们与前两个阶段的延续关系。

## 这一阶段最重要的理解点

读这一阶段时，建议重点抓下面三件事：

1. 为什么 Prompt 不能一直写死在代码里。
2. 为什么“能力组织”会比“继续加工具”更重要。
3. 为什么一个长期演进的 Agent 项目，需要把行为拆成主 prompt、技能、命令、工具这些层。

如果这三点没有抓住，就很容易把 Stage 3 看成“文件变多了”，而不是“结构开始变清楚了”。

## 读代码时建议关注什么

优先关注下面几块：

- `internal/agent/agent.go`
- `internal/skill/skill.go`
- `internal/prompt/loader.go`
- `internal/tools/task_tool.go`
- `internal/tools/skill_tool.go`
- `internal/commands/*.go`

读的时候可以带着这些问题：

- Agent 在什么时候加载和注入 Skill？
- Prompt 改成文件后，带来了什么维护上的好处？
- 任务和技能为什么开始有单独的工具和命令入口？
- 现在的 Agent，和前两个阶段相比，已经更像“一个可扩展系统”了吗？

## 这一阶段容易让新读者困惑的地方

这一阶段开始出现很多名词，容易让人觉得信息量突然变大。读的时候建议刻意分层：

- 先把 `Prompt` 和 `Skill` 理解成“控制 Agent 行为的规则”
- 再把 `Tool` 理解成“Agent 真正拿来做事的能力”
- 最后再看 `Task` 和命令入口，它们更像是“把行为组织起来的外层操作方式”

不要一开始就试图把所有工具名、命令名、技能格式全部背下来。

## 进入下一阶段前，你应该已经理解

- 为什么项目会从“工具越来越多”走向“行为组织越来越重要”
- 为什么 Prompt 和 Skill 的拆分，会让 Agent 更容易扩展和维护
- 为什么 Stage 3 是从“功能累加”走向“结构整理”的关键一步

理解这些之后，再切到 `learn/stage-4-mcp-session-tui`，去看项目怎样开始向更完整的使用体验和扩展能力继续推进。
