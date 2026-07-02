# Claude Context 分片

## 概述

> `claude-context` 是一个面向 AI coding agent 的 MCP 插件。它不把整个仓库一次性塞进模型上下文，而是先把代码切成多个 chunk，做 embedding，再写入 Milvus / Zilliz Cloud 这类向量库；查询时只取相关 chunk 回填给模型。

> 这套设计里，代码分片是索引链路的前半段。分得太粗，会把无关代码一起送去 embedding；分得太碎，会丢掉函数、类、方法这类结构语义。

## 入口

> 分片入口在 `packages/core/src/context.ts`。

> `Context.indexCodebase()` 在扫描到代码文件后，会按扩展名推断语言，然后调用：
>
> ```ts
> const chunks = await this.codeSplitter.split(content, language, filePath);
> ```
>
> 之后这些 `chunks` 会进入 `processChunkBatch()`，先做 `embedBatch()`，再写入向量库。

## 默认分片器

> 默认分片器是 `AstCodeSplitter`，初始化位置也在 `packages/core/src/context.ts`：
>
> ```ts
> this.codeSplitter = config.codeSplitter || new AstCodeSplitter(2500, 300);
> ```
>
> 这里的默认参数是：
> - `chunkSize = 2500`
> - `chunkOverlap = 300`
>
> 这两个值在实现里按字符长度生效，不是按 token 生效。

## AST 分片

> AST 分片实现位于 `packages/core/src/splitter/ast-splitter.ts`。

> 它先用 `tree-sitter` 解析源码，再从 AST 中提取“适合独立检索的逻辑代码单元”。核心思路不是定长切块，而是优先沿语义边界切块。

> `tree-sitter` 可以把源码解析成语法树。`claude-context` 借它找到函数、类、方法、类型声明等节点，再把这些节点对应的源码片段直接作为 chunk。

## 支持语言与节点类型

> `AstCodeSplitter` 内部维护了一组 `SPLITTABLE_NODE_TYPES`。不同语言有不同的可切节点。

> 典型映射如下 ：
> - JavaScript / TypeScript: `function_declaration`、`class_declaration`、`method_definition`、`export_statement`
> - Python: `function_definition`、`class_definition`、`decorated_definition`、`async_function_definition`
> - Go: `function_declaration`、`method_declaration`、`type_declaration`、`var_declaration`、`const_declaration`
> - Rust: `function_item`、`impl_item`、`struct_item`、`enum_item`、`trait_item`、`mod_item`
> - Java / C++ / C# / Scala 也各自定义了方法、类、接口、构造器等节点类型

> 语言映射在 `getLanguageConfig()` 里完成。支持的主要语言有：`javascript`、`typescript`、`python`、`java`、`cpp`、`go`、`rust`、`csharp`、`scala`，以及它们的一些扩展名别名，如 `js`、`ts`、`py`、`rs`、`cs`。

## Chunk 生成方式

> 真正抽取 chunk 的逻辑在 `extractChunks()`。

> 它会深度遍历 AST；当节点类型命中 `splittableTypes` 时，就取该节点的源码范围：
> - `startLine = currentNode.startPosition.row + 1`
> - `endLine = currentNode.endPosition.row + 1`
> - `content = code.slice(currentNode.startIndex, currentNode.endIndex)`
>
> 生成的每个 `CodeChunk` 都会带元数据：
> - `startLine`
> - `endLine`
> - `language`
> - `filePath`

> 这意味着检索结果不是“文件级命中”，而是“文件中的某一段结构化代码命中”。后续展示给模型时，可以直接带上代码片段和行号。

## 大块二次拆分

> AST 命中的块不一定足够小。比如一个超大的类、一个很长的模块、或者一个聚合了很多方法的实现块，都可能超过 `chunkSize`。

> 这时会进入 `refineChunks()`。如果 `chunk.content.length > this.chunkSize`，代码会调用 `splitLargeChunk()` 再拆一次。

> `splitLargeChunk()` 的策略比较直接：按行累加，直到当前子块长度即将超过 `chunkSize`，就落一个新子块。也就是说：
> - 第一层切分优先看 AST 语义边界
> - 第二层切分才退化为按行控制块大小

## Overlap

> 在 `refineChunks()` 末尾，所有块还会经过 `addOverlap()`。

> 它会把“前一个 chunk 末尾的最后 `chunkOverlap` 个字符”拼接到“当前 chunk 的开头”。默认就是 300 个字符。

> 这样做的目的不是增加召回范围，而是保留跨块的局部上下文。比如某个方法开头依赖上一个块的注释、泛型定义、辅助函数尾部，overlap 可以减少语义断裂。

> 这一步还会回调 `metadata.startLine`，尽量让行号范围继续覆盖这段补进来的前文。

## Fallback 到 LangChain

> AST splitter 不是唯一方案。`claude-context` 还实现了 `LangChainCodeSplitter`，代码在 `packages/core/src/splitter/langchain-splitter.ts`。

> AST 会在以下情况回退：
> - 当前语言不在 AST 支持列表里
> - `tree-sitter` 解析失败
> - `tree.rootNode` 不可用
> - AST 分片过程抛异常

> 回退后会调用 `LangChainCodeSplitter.split()`。

> LangChain 是一个常见的 LLM 应用开发库。这里它主要被当成“现成的文本切分器”使用，而不是当成 agent 框架使用。

> `LangChainCodeSplitter` 优先尝试：
>
> ```ts
> RecursiveCharacterTextSplitter.fromLanguage(mappedLanguage, {
>   chunkSize,
>   chunkOverlap,
> })
> ```
>
> 如果语言也不在 LangChain 的支持集合里，就继续退化到通用 `RecursiveCharacterTextSplitter`。

> 这层 fallback 的本质是：即使拿不到 AST，也至少要保证代码能被稳定切块和索引，而不是整条链路失败。

## 没有 AST 命中时

> `extractChunks()` 还有一个兜底分支：如果整棵树遍历后，一个可分节点都没找到，它会把整个文件作为一个 chunk 返回。

> 这说明它的整体容错顺序是：
> 1. 先按 AST 结构切
> 2. AST 结构块太大，就按行再拆
> 3. AST 没打出任何块，就直接整文件返回
> 4. AST 链路出错，就回退到 LangChain 字符分片

## 分片结果如何进入向量库

> `Context.processChunkBatch()` 会把 chunk 内容组装成 `chunkContents`，调用 `this.embedding.embedBatch(chunkContents)` 生成向量。

> 这里的 embedding provider 默认通常是 OpenAI embedding，也可以换成别的实现。embedding 的作用是把一段代码映射成一个高维向量，后续查询时可以按语义相似度做近邻检索。

> Milvus 是一个向量数据库。`claude-context` 会把每个 chunk 的：
> - `content`
> - `vector`
> - `relativePath`
> - `startLine`
> - `endLine`
> - `fileExtension`
> - `metadata`
>
> 一起写进去。这样搜索结果既能按向量相似度召回，也能回到具体文件和代码位置。

## 总结

> `claude-context` 的分片策略不是“平均切成若干段”，而是“语义优先，长度受控，失败可退化”。

> 可以把它概括成四句话：
> - 先按 AST 中的函数、类、方法、类型声明切
> - 块太大，再按行拆到 `chunkSize` 以内
> - 每块追加固定 `chunkOverlap`
> - AST 不可用时，回退到 LangChain 字符分片

> 这也是它适合代码检索的原因：相比纯字符切块，它更容易把一个完整的逻辑单元作为检索对象保留下来。
