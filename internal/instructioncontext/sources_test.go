package instructioncontext

import (
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestDiscoverUserSourcesOnlyReturnsDetectedResources(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("claude context"), 0644); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, home, ".agents", "review", "review", "Review workflow", "body")
	values, err := DiscoverUserSources(home, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("sources = %#v", values)
	}
	if values[0].Provider != "agents" || values[0].Kind != string(instructionpolicy.ResourceSkills) || !values[0].Enabled {
		t.Fatalf("agents source = %#v", values[0])
	}
	if values[1].Provider != "claude" || values[1].Kind != string(instructionpolicy.ResourceContext) || !values[1].Enabled {
		t.Fatalf("claude source = %#v", values[1])
	}
}

func TestDiscoverUserSourcesKeepsDetectedDisabledResourceVisible(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.WriteFile(path, []byte("claude context"), 0644); err != nil {
		t.Fatal(err)
	}
	disabled := false
	policy := instructionpolicy.DefaultConfig()
	policy.Sources["claude"] = instructionpolicy.SourcePolicy{Context: &disabled}
	values, err := DiscoverUserSources(home, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Provider != "claude" || values[0].Enabled || values[0].Loaded || len(values[0].Paths) != 1 || values[0].Paths[0] != path {
		t.Fatalf("sources = %#v", values)
	}
}

func TestDiscoverUserSourcesIncludesNativeCGM(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDir, "skills", "ship-it"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "skills", "ship-it", "SKILL.md"), []byte("---\nname: ship-it\ndescription: Ship\n---\nbody\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := DiscoverUserSources(home, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Provider != "cgm" || values[0].Kind != string(instructionpolicy.ResourceSkills) || !values[0].Enabled {
		t.Fatalf("native sources = %#v", values)
	}
}
