package context

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIncludesProviderRules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("agents"), 0644); err != nil {
		t.Fatal(err)
	}
	rulesDir := filepath.Join(root, ".cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "typescript.mdc"), []byte("rule"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := Load(root, 3, 60000)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 2 {
		t.Fatalf("count = %d, files = %#v", result.Count, result.Files)
	}
}

func TestLoadTruncates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("abcdef"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := Load(root, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Content != "abc" || !result.Files[0].Truncated {
		t.Fatalf("files = %#v", result.Files)
	}
}
