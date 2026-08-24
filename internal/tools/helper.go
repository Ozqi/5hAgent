package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/cloudwego/eino/schema"
)

func resolvePath(root string, path string) string {
	// 这里只提供路径解析便利，不校验结果是否仍位于 root 内，也不解析符号链接。
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
