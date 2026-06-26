package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillsProjectOverridesGlobal(t *testing.T) {
	root := t.TempDir()
	globalDir := filepath.Join(root, "global")
	projectDir := filepath.Join(root, "project")
	writeSkill(t, globalDir, "debugging", `---
name: debugging
description: global debugging
---
global body
`)
	writeSkill(t, projectDir, "debugging", `---
name: debugging
description: project debugging
---
project body
`)
	writeSkill(t, globalDir, "planning", `---
name: planning
description: global planning
---
planning body
`)

	mgr := NewManagerFromDirs(
		Source{Scope: "global", Dir: globalDir},
		Source{Scope: "project", Dir: projectDir},
	)
	if err := mgr.LoadSkills(); err != nil {
		t.Fatalf("LoadSkills() error = %v", err)
	}

	debugging, ok := mgr.GetSkill("debugging")
	if !ok {
		t.Fatalf("debugging skill not loaded")
	}
	if debugging.Scope != "project" || debugging.Description != "project debugging" || debugging.Content != "project body" {
		t.Fatalf("debugging = %#v, want project override", debugging)
	}
	planning, ok := mgr.GetSkill("planning")
	if !ok {
		t.Fatalf("planning skill not loaded")
	}
	if planning.Scope != "global" || planning.Content != "planning body" {
		t.Fatalf("planning = %#v, want global skill", planning)
	}
}

func writeSkill(t *testing.T, root string, name string, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create skill dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill failed: %v", err)
	}
}
