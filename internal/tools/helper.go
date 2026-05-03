// helper.go - 工具辅助函数
// 功能：通用的 JSON 输出和错误处理
package tools

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// JSONResult 将任意值序列化为 schema.ToolResult
func JSONResult(v interface{}) (*schema.ToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}
	return &schema.ToolResult{Parts: []schema.ToolOutputPart{{Type: schema.ToolPartTypeText, Text: string(data)}}}, nil
}

// JSONResultOK 成功结果的便捷封装
func JSONResultOK(v interface{}) *schema.ToolResult {
	r, _ := JSONResult(v) // 只用于已知合法的类型
	return r
}

// EmptyResult 返回空结果的 ToolResult
func EmptyResult() *schema.ToolResult {
	r, _ := JSONResult(map[string]interface{}{})
	return r
}
