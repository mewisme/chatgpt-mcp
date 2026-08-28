package skills

import (
	"os"
	"path/filepath"
	"testing"
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
