// skill.go - 技能加载与管理
// 功能：启动时从全局和项目 skills 目录加载技能，形成 Agent 生命周期内固定的技能集合
// 主要类型：Skill, Manager
// 导出函数：NewManager, NewManagerFromDirs, LoadSkills, GetSkill, ListSkills
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill 定义一个可注入的技能
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version,omitempty"`
	Tools       string `yaml:"tools,omitempty"`
	Content     string `yaml:"-"` // Markdown 内容（不在 frontmatter 中）
	Enabled     bool   `yaml:"-"` // 启动加载状态；当前 Agent 生命周期内不变
	Scope       string `yaml:"-"` // global 或 project，表示来源层级
	Path        string `yaml:"-"` // SKILL.md 绝对或启动目录相对路径
}

// Manager 管理启动时加载出的技能快照
type Manager struct {
	skills    map[string]*Skill
	skillsDir string   // 兼容旧调用；等价于 skillDirs[0]
	skillDirs []Source // 按顺序加载，后面的同名 skill 覆盖前面的
}

// Source 描述一个 skill 来源目录
type Source struct {
	Scope string // global 或 project
	Dir   string // skills 根目录
}

// NewManager 创建技能管理器
func NewManager(skillsDir string) *Manager {
	return NewManagerFromDirs(Source{Scope: "global", Dir: skillsDir})
}

// NewManagerFromDirs 创建支持多来源目录的技能管理器
func NewManagerFromDirs(sources ...Source) *Manager {
	firstDir := ""
	if len(sources) > 0 {
		firstDir = sources[0].Dir
	}
	return &Manager{
		skills:    make(map[string]*Skill),
		skillsDir: firstDir,
		skillDirs: sources,
	}
}

// LoadSkills 从配置的目录加载所有技能；后加载的项目 skill 可覆盖同名全局 skill
func (m *Manager) LoadSkills() error {
	sources := m.skillDirs
	if len(sources) == 0 && m.skillsDir != "" {
		sources = []Source{{Scope: "global", Dir: m.skillsDir}}
	}
	for _, source := range sources {
		if err := m.loadDir(source); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) loadDir(source Source) error {
	if source.Dir == "" {
		return nil
	}
	if _, err := os.Stat(source.Dir); os.IsNotExist(err) {
		return nil
	}
	entries, err := os.ReadDir(source.Dir)
	if err != nil {
		return fmt.Errorf("failed to read skills dir %s: %w", source.Dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(source.Dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			continue
		}
		skill, err := m.loadSkillFile(skillPath)
		if err != nil {
			continue
		}
		skill.Scope = source.Scope
		skill.Path = skillPath
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
	skill.Enabled = true

	return &skill, nil
}

// GetSkill 获取指定技能
// GetSkill retrieves a skill by name. Returns (skill, true) if found, (nil, false) otherwise.
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
