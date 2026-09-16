package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestDiscoverAcrossProviders(t *testing.T) {
	root := t.TempDir()
	for _, provider := range []string{".agents", ".claude", ".cursor"} {
		dir := filepath.Join(root, provider, "skills", provider[1:]+"-skill")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + provider[1:] + "\ndescription: provider skill\n---\n# Skill\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	values, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 {
		t.Fatalf("skills = %#v", values)
	}
	loaded, err := Load(root, "cursor", 200000)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Skill.Source != ".cursor" || loaded.Truncated {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestDiscoverNativeCGMAndWorkspaceOverridesGlobal(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	workspace := t.TempDir()
	home := t.TempDir()
	writeNativeSkill(t, filepath.Join(configDir, "skills", "ship-it"), "ship-it", "global ship")
	writeNativeSkill(t, filepath.Join(workspace, ".cgm", "skills", "ship-it"), "ship-it", "workspace ship")
	writeNativeSkill(t, filepath.Join(configDir, "skills", "global-only"), "global-only", "global only")
	values, err := DiscoverWithUser(workspace, home, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Skill{}
	for _, skill := range values {
		byName[skill.Name] = skill
	}
	if byName["ship-it"].Description != "workspace ship" || byName["ship-it"].Source != NativeSource {
		t.Fatalf("workspace override = %#v", byName["ship-it"])
	}
	if byName["global-only"].Source != NativeSource || !strings.HasPrefix(byName["global-only"].Path, configDir) {
		t.Fatalf("global skill = %#v", byName["global-only"])
	}
	loaded, err := LoadWithUser(workspace, home, "ship-it", 200000, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Skill.Description != "workspace ship" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestDiscoverWithUserIncludesBuiltinsAndReservesNames(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	workspace := t.TempDir()
	home := t.TempDir()
	writeNativeSkill(t, filepath.Join(workspace, ".agents", "skills", "create-skill"), "create-skill", "shadow")
	values, err := DiscoverWithUser(workspace, home, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Skill{}
	for _, skill := range values {
		byName[skill.Name] = skill
	}
	if !Builtin(byName["create-skill"]) || !Builtin(byName["create-rule"]) {
		t.Fatalf("builtins = %#v", values)
	}
	if byName["create-skill"].Description == "shadow" {
		t.Fatal("builtin name shadowed")
	}
	loaded, err := LoadWithUser(workspace, home, "create-skill", 200000, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.Content, "create_skill") || !strings.Contains(loaded.Content, "Do not fall back") {
		t.Fatalf("builtin body = %q", loaded.Content)
	}
}

func writeNativeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n# Skill\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
