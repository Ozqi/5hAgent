package logger

import (
	"fmt"
	"strings"
)

// ToolPrinter 工具调用的格式化输出
type ToolPrinter struct {
	indent string
}

// NewToolPrinter 创建新的工具打印器
func NewToolPrinter() *ToolPrinter {
	return &ToolPrinter{
		indent: "  ",
	}
}

// PrintToolCall 打印工具调用
// 格式: ● ToolName(args...)
func (p *ToolPrinter) PrintToolCall(name string, args string, concurrent bool) {
	// 截断参数显示
	argsDisplay := TruncateString(args, 80)

	mode := ""
	if concurrent {
		mode = " [并发]"
	}

	fmt.Printf("\n● %s(%s)%s\n", Cyan(name), Gray(argsDisplay), Gray(mode))
}

// PrintToolResult 打印工具执行结果
// 格式: ⎿ result
func (p *ToolPrinter) PrintToolResult(result string) {
	// 截断结果显示
	resultDisplay := TruncateString(result, 150)

	// 如果结果为空
	if strings.TrimSpace(result) == "" {
		fmt.Printf("%s⎿ %s\n", p.indent, Gray("(无输出)"))
		return
	}

	// 如果结果是多行，只显示第一行
	lines := strings.Split(resultDisplay, "\n")
	if len(lines) > 1 {
		fmt.Printf("%s⎿ %s\n", p.indent, lines[0])
		for i := 1; i < len(lines) && i < 3; i++ {
			fmt.Printf("%s   %s\n", p.indent, lines[i])
		}
		if len(lines) > 3 {
			fmt.Printf("%s   %s\n", p.indent, Gray("..."))
		}
	} else {
		fmt.Printf("%s⎿ %s\n", p.indent, resultDisplay)
	}
}

// PrintToolError 打印工具执行错误
// 格式: ⎿ ✗ error
func (p *ToolPrinter) PrintToolError(err error) {
	fmt.Printf("%s⎿ %s %s\n", p.indent, Red("✗"), Red(err.Error()))
}

// PrintToolStatus 打印工具状态信息
// 格式: ⎿ status message
func (p *ToolPrinter) PrintToolStatus(message string) {
	fmt.Printf("%s⎿ %s\n", p.indent, Gray(message))
}

// PrintSummary 打印工具执行汇总
// 格式: Searched for N patterns, read M files (ctrl+o to expand)
func (p *ToolPrinter) PrintSummary(message string) {
	fmt.Printf("\n%s%s\n", p.indent, Gray(message))
}

// Global instance
var defaultToolPrinter = NewToolPrinter()

// PrintToolCall 全局函数：打印工具调用
func PrintToolCall(name string, args string, concurrent bool) {
	defaultToolPrinter.PrintToolCall(name, args, concurrent)
}

// PrintToolResult 全局函数：打印工具结果
func PrintToolResult(result string) {
	defaultToolPrinter.PrintToolResult(result)
}

// PrintToolError 全局函数：打印工具错误
func PrintToolError(err error) {
	defaultToolPrinter.PrintToolError(err)
}

// PrintToolStatus 全局函数：打印工具状态
func PrintToolStatus(message string) {
	defaultToolPrinter.PrintToolStatus(message)
}

// PrintToolSummary 全局函数：打印工具汇总
func PrintToolSummary(message string) {
	defaultToolPrinter.PrintSummary(message)
}
