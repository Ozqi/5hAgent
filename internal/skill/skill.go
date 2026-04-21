package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Skill 定义一个可注入的技能
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Enabled     bool   `json:"enabled"`
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

// LoadSkills 从目录加载所有技能
func (m *Manager) LoadSkills() error {
	if m.skillsDir == "" {
		return nil
	}

	if _, err := os.Stat(m.skillsDir); os.IsNotExist(err) {
		return nil
	}

	files, err := filepath.Glob(filepath.Join(m.skillsDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to glob skills: %w", err)
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var skill Skill
		if err := json.Unmarshal(data, &skill); err != nil {
			continue
		}

		m.skills[skill.Name] = &skill
	}

	return nil
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

// InjectSkills 将启用的技能注入到 system prompt
func (m *Manager) InjectSkills(basePrompt string) string {
	if len(m.skills) == 0 {
		return basePrompt
	}

	injected := basePrompt + "\n\n# Available Skills\n"
	for _, skill := range m.skills {
		if skill.Enabled {
			injected += fmt.Sprintf("\n## %s\n%s\n\n%s\n", skill.Name, skill.Description, skill.Prompt)
		}
	}

	return injected
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
