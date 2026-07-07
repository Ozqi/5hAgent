// helper.go - 工具辅助函数
// 功能：通用的 JSON 输出和错误处理
package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/cloudwego/eino/schema"
)

func resolvePath(root string, path string) string {
	if path == "" {
		path = "."
	}
	if filepath.IsAbs(path) || root == "" {
		return path
	}
	return filepath.Join(root, path)
}

// JSONResult 将任意值序列化为 schema.ToolResult
func JSONResult(v interface{}) (*schema.ToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}
	return &schema.ToolResult{Parts: []schema.ToolOutputPart{{Type: schema.ToolPartTypeText, Text: string(data)}}}, nil
}
