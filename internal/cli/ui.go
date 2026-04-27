// ui.go - 简单终端输出
// 功能：提供基础错误输出
// 导出函数：PrintError
package cli

import (
	"fmt"
)

// PrintError prints error messages in red (using ANSI color codes)
func PrintError(err error) {
	fmt.Printf("\033[31mError: %v\033[0m\n", err)
}
