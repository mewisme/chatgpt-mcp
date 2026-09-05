package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertWorkspaceMemoryUsesScopedMarkdownAndReplacesSameScope(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	path, err := store.Upsert("ws_test", "tooling", "use pnpm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, filepath.Join("ws_test", "MEMORY.md")) {
		t.Fatalf("path = %q", path)
	}
	content, err := store.Load("ws_test")
	if err != nil {
		t.Fatal(err)
	}
	if content != "## tooling\n- use pnpm" {
		t.Fatalf("content = %q", content)
	}
	if _, err := store.Upsert("ws_test", "tooling", "use pnpm with Corepack"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert("ws_test", "tui", "use Charm defaults"); err != nil {
		t.Fatal(err)
	}
	content, err = store.Load("ws_test")
	if err != nil {
		t.Fatal(err)
	}
	if content != "## tooling\n- use pnpm with Corepack\n\n## tui\n- use Charm defaults" || strings.Count(content, "## tooling") != 1 {
		t.Fatalf("content = %q", content)
	}
}

func TestUpsertWorkspaceMemoryMigratesLegacyDateNotesToGeneral(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	path := store.WorkspacePath("ws_test")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := "# Auto memory (cross-session notes)\n\n- 2026-09-04: use compact imports\n- 2026-09-05: prefer pnpm\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert("ws_test", "tui", "use Charm defaults"); err != nil {
		t.Fatal(err)
	}
	content, err := store.Load("ws_test")
	if err != nil {
		t.Fatal(err)
	}
	want := "## general\n- use compact imports prefer pnpm\n\n## tui\n- use Charm defaults"
	if content != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}
