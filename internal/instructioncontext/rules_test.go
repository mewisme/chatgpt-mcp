package instructioncontext

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRuleFile(t *testing.T, root, provider, name, content string) string {
	t.Helper()
	path := filepath.Join(root, provider, "rules", name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadUnconditionalRulesFiltersScopedRulesAndPrefersAgents(t *testing.T) {
	root := t.TempDir()
	agentsPath := writeRuleFile(t, root, ".agents", "primary.md", "agents unconditional")
	writeRuleFile(t, root, ".claude", "duplicate.md", "agents unconditional")
	claudesPath := writeRuleFile(t, root, ".claudes", "always.md", "---\npaths: [\"src/**/*.ts\"]\nalwaysApply: true\n---\nclaudes always")
	writeRuleFile(t, root, ".cursor", "scoped.mdc", "---\nglobs: [\"src/**/*.tsx\"]\n---\ncursor scoped")
	codexPath := writeRuleFile(t, root, ".codex", "fallback.md", "codex unconditional")

	loaded, err := LoadUnconditionalRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 {
		t.Fatalf("rules = %#v", loaded)
	}
	if loaded[0].Path != agentsPath || loaded[0].Source != ".agents" || loaded[0].Content != "agents unconditional" {
		t.Fatalf("primary rule = %#v", loaded[0])
	}
	if loaded[1].Path != claudesPath || loaded[1].Source != ".claudes" || !loaded[1].AlwaysApply || len(loaded[1].Patterns) == 0 {
		t.Fatalf("always rule = %#v", loaded[1])
	}
	if loaded[2].Path != codexPath || loaded[2].Source != ".codex" {
		t.Fatalf("fallback rule = %#v", loaded[2])
	}
}

func TestLoadUnconditionalRulesLoadsRuleWithoutPatterns(t *testing.T) {
	root := t.TempDir()
	path := writeRuleFile(t, root, ".agents", "plain.md", "---\ndescription: global\n---\nplain global rule")
	loaded, err := LoadUnconditionalRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Path != path || loaded[0].AlwaysApply || len(loaded[0].Patterns) != 0 {
		t.Fatalf("rules = %#v", loaded)
	}
}

func TestLoadUnconditionalRulesSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside rule"), 0644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".agents", "rules")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	loaded, err := LoadUnconditionalRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("rules = %#v", loaded)
	}
}
