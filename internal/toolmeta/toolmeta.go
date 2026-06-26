// toolmeta.go - 工具元数据注册
// 功能：工具分类（base/task/skill/mcp）、只读属性、显示名称
// 主要类型：Meta, Category
// 导出函数：Reset, Register, Lookup, DisplayName, IsReadOnly
package toolmeta

import (
	"strings"
	"sync"
)

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

var (
	mu       sync.RWMutex
	registry = map[string]Meta{}
)

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Meta{}
}

func Register(meta Meta) {
	if meta.FullName == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	registry[meta.FullName] = meta
}

func Lookup(name string) (Meta, bool) {
	mu.RLock()
	defer mu.RUnlock()
	meta, ok := registry[name]
	return meta, ok
}

func DisplayName(name string) string {
	if meta, ok := Lookup(name); ok && meta.DisplayName != "" {
		return meta.DisplayName
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return name
}

func IsReadOnly(name string) bool {
	if meta, ok := Lookup(name); ok {
		return meta.ReadOnly
	}
	switch name {
	case "base.read_file", "base.glob", "base.grep", "base.list_dir":
		return true
	}
	return false
}
