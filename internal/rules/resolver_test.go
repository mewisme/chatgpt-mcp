package rules

import (
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestLoadRulesAcrossProviders(t *testing.T) {
	root := t.TempDir()
	for _, provider := range []string{".claude", ".agents", ".cursor"} {
		dir := filepath.Join(root, provider, "rules")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		content := "---\npaths:\n  - \"src/**/*.ts\"\n---\nUse TypeScript rule from " + provider
		if provider == ".cursor" {
			content = "---\nglobs: [\"src/**/*.ts\", \"*.tsx\"]\n---\nCursor rule"
		}
		if err := os.WriteFile(filepath.Join(dir, "rule.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(root, "src", "app", "page.ts")
	values, err := LoadForFile(root, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 {
		t.Fatalf("rules = %#v", values)
	}
}

func TestAlwaysApplyRule(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".cursor", "rules")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "always.mdc"), []byte("---\nalwaysApply: true\n---\nAlways"), 0644); err != nil {
		t.Fatal(err)
	}
	values, err := LoadForFile(root, filepath.Join(root, "anything.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || !values[0].AlwaysApply {
		t.Fatalf("rules = %#v", values)
	}
}

func TestLoadNativeCGMRules(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	workspace := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".cgm", "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".cgm", "rules", "typescript.md"), []byte("---\nglobs: [\"**/*.ts\"]\n---\nWorkspace TypeScript"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "rules", "always.md"), []byte("---\nalwaysApply: true\n---\nGlobal always"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "src", "app.ts")
	values, err := LoadForFileWithUser(workspace, target, home, instructionpolicy.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("rules = %#v", values)
	}
}
