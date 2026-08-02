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

func TestReloadSkillsReplacesOnlyValidSnapshot(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "first", "---\nname: first\ndescription: first\n---\nbody\n")
	mgr := NewManager(root)
	if err := mgr.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(root, "first")); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, root, "second", "---\nname: second\ndescription: second\n---\nbody\n")
	if err := mgr.ReloadSkills(); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.GetSkill("first"); ok {
		t.Fatal("deleted skill still present after reload")
	}
	if _, ok := mgr.GetSkill("second"); !ok {
		t.Fatal("new skill missing after reload")
	}

	writeSkill(t, root, "broken", "missing frontmatter")
	if err := mgr.ReloadSkills(); err == nil {
		t.Fatal("ReloadSkills() error = nil for invalid skill")
	}
	if _, ok := mgr.GetSkill("second"); !ok {
		t.Fatal("failed reload replaced previous valid snapshot")
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
