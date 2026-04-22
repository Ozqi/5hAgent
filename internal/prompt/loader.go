package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Prompt 提示词结构
type Prompt struct {
	Name        string            `yaml:"name"`        // 提示词名称
	Description string            `yaml:"description"` // 提示词描述
	Version     string            `yaml:"version"`     // 版本号
	Type        string            `yaml:"type"`        // 类型: system/user/assistant
	Variables   map[string]string `yaml:"variables"`   // 变量定义（可选）
	Content     string            `yaml:"-"`           // 提示词内容（不在 frontmatter 中）
}

// Loader 提示词加载器
type Loader struct {
	promptsDir string
	prompts    map[string]*Prompt
}

// NewLoader 创建提示词加载器
// 参数:
//   - promptsDir: 提示词目录路径
//
// 返回: Loader 实例
func NewLoader(promptsDir string) *Loader {
	return &Loader{
		promptsDir: promptsDir,
		prompts:    make(map[string]*Prompt),
	}
}

// Load 加载所有提示词文件
// 功能:
//  1. 遍历 prompts 目录
//  2. 加载所有 .md 文件
//  3. 解析 frontmatter 和内容
//
// 返回: 可能的错误
func (l *Loader) Load() error {
	if l.promptsDir == "" {
		return fmt.Errorf("prompts directory not specified")
	}

	if _, err := os.Stat(l.promptsDir); os.IsNotExist(err) {
		return fmt.Errorf("prompts directory does not exist: %s", l.promptsDir)
	}

	// 遍历目录下的所有 .md 文件
	return filepath.Walk(l.promptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和非 .md 文件
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		// 加载提示词文件
		prompt, err := l.loadPromptFile(path)
		if err != nil {
			// 跳过无效文件，继续加载其他文件
			return nil
		}

		l.prompts[prompt.Name] = prompt
		return nil
	})
}

// loadPromptFile 加载单个提示词文件
// 参数:
//   - path: 文件路径
//
// 返回: Prompt 实例和可能的错误
// 功能:
//  1. 读取文件内容
//  2. 解析 frontmatter (YAML)
//  3. 提取提示词内容
func (l *Loader) loadPromptFile(path string) (*Prompt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file: %w", err)
	}

	content := string(data)

	// 检查是否有 frontmatter
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("invalid prompt format: missing frontmatter")
	}

	// 分割 frontmatter 和内容
	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid prompt format: malformed frontmatter")
	}

	// 解析 frontmatter
	var prompt Prompt
	if err := yaml.Unmarshal([]byte(parts[0]), &prompt); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// 提取内容
	prompt.Content = strings.TrimSpace(parts[1])

	return &prompt, nil
}

// Get 获取指定名称的提示词
// 参数:
//   - name: 提示词名称
//
// 返回: Prompt 实例和是否存在
func (l *Loader) Get(name string) (*Prompt, bool) {
	prompt, ok := l.prompts[name]
	return prompt, ok
}

// GetContent 获取提示词内容
// 参数:
//   - name: 提示词名称
//
// 返回: 提示词内容和可能的错误
func (l *Loader) GetContent(name string) (string, error) {
	prompt, ok := l.prompts[name]
	if !ok {
		return "", fmt.Errorf("prompt not found: %s", name)
	}
	return prompt.Content, nil
}

// GetContentWithVars 获取提示词内容并替换变量
// 参数:
//   - name: 提示词名称
//   - vars: 变量映射表
//
// 返回: 替换变量后的提示词内容和可能的错误
// 功能:
//  1. 获取提示词内容
//  2. 替换 {{variable}} 格式的变量
func (l *Loader) GetContentWithVars(name string, vars map[string]string) (string, error) {
	content, err := l.GetContent(name)
	if err != nil {
		return "", err
	}

	// 替换变量
	for key, value := range vars {
		placeholder := fmt.Sprintf("{{%s}}", key)
		content = strings.ReplaceAll(content, placeholder, value)
	}

	return content, nil
}

// List 列出所有提示词
// 返回: 提示词列表
func (l *Loader) List() []*Prompt {
	prompts := make([]*Prompt, 0, len(l.prompts))
	for _, prompt := range l.prompts {
		prompts = append(prompts, prompt)
	}
	return prompts
}

// Reload 重新加载所有提示词
// 返回: 可能的错误
func (l *Loader) Reload() error {
	l.prompts = make(map[string]*Prompt)
	return l.Load()
}
