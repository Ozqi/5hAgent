# 5hAgent 拓扑

本页说明 `doc/topology.json` 和 `doc/topology.mmd` 的分工。

| 文件 | 职责 |
| --- | --- |
| [topology.json](topology.json) | 拓扑事实源，只描述节点、边、视图和结论。 |
| [topology.mmd](topology.mmd) | 从 JSON 派生的 Mermaid 图源。 |

`topology.json` 按 `draw-topology-json` skill 生成，不包含 Mermaid、draw.io 布局或视觉样式。`topology.mmd` 按 `draw-mermaid` skill 从 JSON 渲染，图中不新增拓扑事实。
