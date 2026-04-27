// utils.go - 工具函数
// 功能：加载 prompt 目录下的 .md 文件内容
// 导出函数：Load
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load reads prompt file <dir>/<name>.md and returns its trimmed content.
func Load(dir, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		return "", fmt.Errorf("prompt %q not found: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}
