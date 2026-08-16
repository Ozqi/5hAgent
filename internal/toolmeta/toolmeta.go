// toolmeta.go - 工具元数据类型
// 功能：定义工具分类、只读属性和显示名 fallback。
// 主要类型：Meta, Category
// 导出函数：DisplayNameFallback
package toolmeta

import "strings"

type Category string

const (
	CategoryBase    Category = "base"
	CategoryTask    Category = "task"
	CategorySkill   Category = "skill"
	CategoryContext Category = "context"
	CategorySystem  Category = "sys"
	CategoryMCP     Category = "mcp"
)

type Meta struct {
	Category     Category
	Source       string
	DisplayName  string
	FullName     string
	OriginalName string
	ReadOnly     bool
}

func DisplayNameFallback(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return name
}
