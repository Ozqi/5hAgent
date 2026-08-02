package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lzq/5hAgent/internal/skill"
)

func TestSkillCommandsShowSourceMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "debugging", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: debugging\ndescription: debug safely\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := skill.NewManagerFromDirs(skill.Source{Scope: "project", Dir: root})
	if err := mgr.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	list, err := HandleSkill("/skill list", mgr)
	if err != nil {
		t.Fatal(err)
	}
	get, err := HandleSkill("/skill get debugging", mgr)
	if err != nil {
		t.Fatal(err)
	}
	for name, output := range map[string]string{"list": list, "get": get} {
		if !strings.Contains(output, "project") || !strings.Contains(output, path) {
			t.Fatalf("%s output missing scope/path:\n%s", name, output)
		}
	}
}
