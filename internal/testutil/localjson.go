package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func RepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}

func WriteLocalJSON(t *testing.T, name string, value any) {
	t.Helper()
	dir := filepath.Join(RepoRoot(t), ".local")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
