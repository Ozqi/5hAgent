# forAI

只读
本文档由人工写成，是真个项目的目标，你不可能一次性完成整个工程，要拆分任务一步步实现。具体看task.md并配合git来管理进度。

# miniAgent

用最少的代码量,实现一个高效完备的Agent。

# 设计

./doc为设计文档,也有AI管理.

我暂时能想到的设计：

## Task：

用户提交的任务会被整理为Task，由main管理

种类：

1. Task：用户第一次提交上来的任务，完成以后向用户汇报or持久化。
2. miniTask（当然你也可以改适当名字），最小任务单元，不能再拆分，大模型能一口气一次性很好的完成。
3. 长时间任务，往往伴随着等待，阻塞，周期性工作，而不是特别高调大模型需要思考
4. 长上下文任务，有很多文本的录入需要大模型处理（但是这些文本信息密度不高）比如html文件，超大文档阅读和解析。
   ...
5. 长思考任务, 需要最顶级的模型,以最高的智商解决棘手问题。是Agent的智力极限。

## sub-Agent

只有一个main Agent，他可以编排任务，发起sub-Agent去执行任务。main管理所有的Task状态和sub-Agent状态。

sub-Agent的提示词有模板，由main挑选进行实例化，完成任务后交给main确认，确认完毕后释放提示词，

## 持久化，memory

这部分先参考Claude-code的设计

## debug，日志

开启debug以后，我希望能看到所有的调用大模型API的实际输入输出，以及工具调用，供我debug

## LLMprovider

暂时只使用minimax，
sk-cp-yybpI1wnFpx9lchqPxh-m8cSG1DxL-csYBH7iwkJAS0G6spOvAzhQANTIW1lm0wgziE_nV1kyGGy6_xUa3nZWwpbCKlXQBrpyvv5kuJSa6S69fM1VO_Xj28

# 参考资源

参考openclaw的通讯，ClaudeCode的设计
/home/lzq/Proj/claude-code
/home/lzq/Proj/openclaw
