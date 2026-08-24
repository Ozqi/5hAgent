// Package cli 提供非 TUI 场景下的基础终端输出辅助能力。
package cli

import (
	"fmt"
	"os"
)

// PrintError 使用红色 ANSI 文本将错误写入 stderr。
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
}
