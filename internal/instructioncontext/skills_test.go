package instructioncontext

import (
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func writeSkillFile(t *testing.T, root, provider, dir, name, description, body string) string {
	t.Helper()
	path := filepath.Join(root, provider, "skills", dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSkillSummariesPrefersAgentsAndReturnsMetadataOnly(t *testing.T) {
	root := t.TempDir()
	agentsPath := writeSkillFile(t, root, ".agents", "release", "release", "Release workflow", "SECRET BODY MUST NOT APPEAR")
	claudePath := writeSkillFile(t, root, ".claude", "review", "review", "Review workflow", "review body")
	cursorPath := writeSkillFile(t, root, ".cursor", "release-alt", "release", "Alternative release workflow", "alternate body")

	loaded, err := LoadSkillSummaries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 {
		t.Fatalf("skills = %#v", loaded)
	}
	if loaded[0].Source != ".agents" || loaded[0].Path != agentsPath || loaded[0].Name != "release" || loaded[0].Description != "Release workflow" {
		t.Fatalf("agents skill = %#v", loaded[0])
	}
	if loaded[1].Source != ".claude" || loaded[1].Path != claudePath {
		t.Fatalf("claude skill = %#v", loaded[1])
	}
	if loaded[2].Source != ".cursor" || loaded[2].Path != cursorPath || loaded[2].Name != "release" {
		t.Fatalf("cursor skill = %#v", loaded[2])
	}
	for _, skill := range loaded {
		if skill.Name == "SECRET BODY MUST NOT APPEAR" || skill.Description == "SECRET BODY MUST NOT APPEAR" {
			t.Fatalf("skill body leaked into summary: %#v", skill)
		}
	}
}

func TestLoadSkillSummariesSupportsAllProviders(t *testing.T) {
	root := t.TempDir()
	providers := []string{".agents", ".claude", ".claudes", ".cursor", ".codex"}
	for _, provider := range providers {
		writeSkillFile(t, root, provider, provider[1:], provider[1:], provider+" skill", "body")
	}
	loaded, err := LoadSkillSummaries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(providers) {
		t.Fatalf("skills = %#v", loaded)
	}
	for i, provider := range providers {
		if loaded[i].Source != provider {
			t.Fatalf("skill %d = %#v", i, loaded[i])
		}
	}
}

func TestLoadSkillSummariesPrefersWorkspaceNativeOverGlobal(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	root := t.TempDir()
	home := t.TempDir()
	workspacePath := writeSkillFile(t, root, ".cgm", "ship-it", "ship-it", "Workspace ship", "workspace body")
	if err := os.MkdirAll(filepath.Join(configDir, "skills", "ship-it"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "skills", "ship-it", "SKILL.md"), []byte("---\nname: ship-it\ndescription: Global ship\n---\nglobal body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "skills", "global-only"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "skills", "global-only", "SKILL.md"), []byte("---\nname: global-only\ndescription: Global only\n---\nbody\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, root, ".agents", "review", "review", "Review workflow", "review body")
	loaded, err := LoadSkillSummariesWithUser(root, home, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) < 3 || loaded[0].Path != workspacePath || loaded[0].Source != ".cgm" {
		t.Fatalf("skills = %#v", loaded)
	}
	if loaded[1].Source != ".agents" {
		t.Fatalf("provider after native = %#v", loaded[1])
	}
	if loaded[len(loaded)-1].Name != "global-only" || loaded[len(loaded)-1].Source != ".cgm" {
		t.Fatalf("global last = %#v", loaded)
	}
}

func TestLoadSkillSummariesSkipsSymlinkSkills(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeSkillFile(t, outside, ".agents", "outside", "outside", "Outside skill", "body")
	link := filepath.Join(root, ".agents", "skills", "linked")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, ".agents", "skills", "outside"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	loaded, err := LoadSkillSummaries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("skills = %#v", loaded)
	}
}
