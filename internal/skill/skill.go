// Package skill 从全局和项目目录加载 Skill，并维护可注入 Agent 的技能快照。
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
	Content     string `yaml:"-"` // Markdown 内容（不在 frontmatter 中）
	Scope       string `yaml:"-"` // global 或 project，表示来源层级
	Path        string `yaml:"-"` // SKILL.md 绝对或启动目录相对路径
}

// Manager 管理启动时加载或显式 reload 后的技能快照。
type Manager struct {
	skills    map[string]*Skill
	skillDirs []Source // 按顺序加载，后面的同名 skill 覆盖前面的
}

// Source 描述一个 skill 来源目录
type Source struct {
	Scope string // global 或 project
	Dir   string // skills 根目录
}

// NewManagerFromDirs 创建支持多来源目录的技能管理器
func NewManagerFromDirs(sources ...Source) *Manager {
	return &Manager{
		skills:    make(map[string]*Skill),
		skillDirs: sources,
	}
}

// LoadSkills 从配置的目录加载所有技能；后加载的项目 skill 可覆盖同名全局 skill
func (m *Manager) LoadSkills() error {
	// 来源按配置顺序扫描，后写入 map 的同名 skill 覆盖先前来源。
	for _, source := range m.skillDirs {
		if err := m.loadDir(source, false); err != nil {
			return err
		}
	}
	return nil
}

// ReloadSkills 先完整构建新快照，扫描成功后才替换当前 skills。
func (m *Manager) ReloadSkills() error {
	// 使用临时 Manager 隔离半成品，所有目录严格加载成功后再整体替换。
	next := NewManagerFromDirs(m.skillDirs...)
	for _, source := range next.skillDirs {
		if err := next.loadDir(source, true); err != nil {
			return err
		}
	}
	m.skills = next.skills
	return nil
}

func (m *Manager) loadDir(source Source, strict bool) error {
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
	// 目录项和 SKILL.md 均来自本地信任边界；普通加载跳过坏文件，reload 则整体失败。
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
			if strict {
				return fmt.Errorf("load skill %s: %w", skillPath, err)
			}
			continue
		}
		skill.Scope = source.Scope
		skill.Path = skillPath
		m.skills[skill.Name] = skill
	}
	return nil
}

// loadSkillFile 解析单个 SKILL.md 的 YAML frontmatter 和待注入 Markdown 正文。
func (m *Manager) loadSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	content := string(data)

	// 先切分 frontmatter，再解析元数据；正文保留为后续模型上下文注入内容。
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

	return &skill, nil
}

// GetSkill 按名称获取 skill，找到时返回 (skill, true)，否则返回 (nil, false)。
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
