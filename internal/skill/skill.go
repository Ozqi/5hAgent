package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// skill.go - Skill 核心逻辑
// 职责：
//   1. 定义 Skill 数据结构
//   2. 从文件系统加载 SKILL.md 文件
//   3. 管理 skill 的启用/禁用状态
//   4. 提供 skill 查询接口
//
// 注意：Skill 的使用（注入、命令处理）在 internal/agent/skill_handler.go 中

// Skill 定义一个可注入的技能
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version,omitempty"`
	Tools       string `yaml:"tools,omitempty"`
	Content     string `yaml:"-"` // Markdown 内容（不在 frontmatter 中）
	Enabled     bool   `yaml:"-"` // 运行时状态
}

// Manager 管理技能的加载和注入
type Manager struct {
	skills    map[string]*Skill
	skillsDir string
}

// NewManager 创建技能管理器
func NewManager(skillsDir string) *Manager {
	return &Manager{
		skills:    make(map[string]*Skill),
		skillsDir: skillsDir,
	}
}

// LoadSkills 从目录加载所有技能（SKILL.md 格式）
func (m *Manager) LoadSkills() error {
	if m.skillsDir == "" {
		return nil
	}

	if _, err := os.Stat(m.skillsDir); os.IsNotExist(err) {
		return nil
	}

	// 遍历 skills 目录下的所有子目录
	entries, err := os.ReadDir(m.skillsDir)
	if err != nil {
		return fmt.Errorf("failed to read skills dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(m.skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			continue
		}

		skill, err := m.loadSkillFile(skillPath)
		if err != nil {
			continue
		}

		m.skills[skill.Name] = skill
	}

	return nil
}

// loadSkillFile 加载单个 SKILL.md 文件
func (m *Manager) loadSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	content := string(data)

	// 解析 frontmatter
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("invalid skill format: missing frontmatter")
	}

	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid skill format: malformed frontmatter")
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(parts[0]), &skill); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	skill.Content = strings.TrimSpace(parts[1])
	skill.Enabled = false // 默认禁用

	return &skill, nil
}

// GetSkill 获取指定技能
func (m *Manager) GetSkill(name string) (*Skill, bool) {
	skill, ok := m.skills[name]
	return skill, ok
}

// ListSkills 列出所有技能
func (m *Manager) ListSkills() []*Skill {
	skills := make([]*Skill, 0, len(m.skills))
	for _, skill := range m.skills {
		skills = append(skills, skill)
	}
	return skills
}

// EnableSkill 启用技能
func (m *Manager) EnableSkill(name string) error {
	skill, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}
	skill.Enabled = true
	return nil
}

// DisableSkill 禁用技能
func (m *Manager) DisableSkill(name string) error {
	skill, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}
	skill.Enabled = false
	return nil
}

// InjectSkills 已废弃：技能现在作为独立消息注入，不再混入 system prompt
// 保留此方法以保持向后兼容
func (m *Manager) InjectSkills(basePrompt string) string {
	return basePrompt
}
