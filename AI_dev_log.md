# 二阶段，

刚才我们实现了流式输出,但是引发了toolcall的问题

原来的错误（已修复）:
// 错误：用索引作为 key  
 toolCallsMap := make(map[string]\*schema.ToolCall)  
 key := fmt.Sprintf("_index_%d", i) // i 是 chunk 内的索引  
 toolCallsMap[key] = tc

问题原因:

- LLM 返回多个工具时，每个 chunk 内的索引都从 0 开始
- 第一个工具：chunk1[0] → \_index_0
- 第二个工具：chunk2[0] → \_index_0 (覆盖了第一个！)
- 结果：两个工具的 arguments 被合并成 {"path":"..."}{"command":"..."}

现在的修复:
// 正确：用列表 + ID 索引
var toolCallsList []\*schema.ToolCall
toolCallsIndex := make(map[string]int) // id -> list index

if tc.ID != "" {  
 // 新工具调用，追加到列表  
 toolCallsList = append(toolCallsList, &tcCopy)
toolCallsIndex[tc.ID] = len(toolCallsList) - 1
} else {  
 // 后续 chunk，合并到最后一个工具
lastTC := toolCallsList[len(toolCallsList)-1]
lastTC.Function.Arguments += tc.Function.Arguments
}

已测试通过并提交到 git (commit d1dafcc)。
