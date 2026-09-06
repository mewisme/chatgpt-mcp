package memory

import (
	"os"
	"path/filepath"
	"testing"
)

type countingIndex struct {
	Index
	rebuilds int
}

func (i *countingIndex) Rebuild(workspaceID string, entries []Entry) error {
	i.rebuilds++
	return i.Index.Rebuild(workspaceID, entries)
}

func TestIndexLifecycleRebuildsOnlyOnFingerprintChange(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	index := &countingIndex{Index: NewMemoryIndex()}
	lifecycle := NewIndexLifecycle(store, index)
	if _, err := store.Upsert("ws_test", "tui", "theme", "Charm"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Ensure("ws_test"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Ensure("ws_test"); err != nil {
		t.Fatal(err)
	}
	if index.rebuilds != 1 {
		t.Fatalf("rebuilds = %d", index.rebuilds)
	}
	if _, err := store.Upsert("ws_test", "tui", "layout", "Center"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Ensure("ws_test"); err != nil {
		t.Fatal(err)
	}
	if index.rebuilds != 2 {
		t.Fatalf("rebuilds = %d", index.rebuilds)
	}
}

func TestIndexLifecycleDetectsManualMemoryEdit(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	index := &countingIndex{Index: NewMemoryIndex()}
	lifecycle := NewIndexLifecycle(store, index)
	if _, err := store.Upsert("ws_test", "tui", "theme", "Charm"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Ensure("ws_test"); err != nil {
		t.Fatal(err)
	}
	path := store.WorkspacePath("ws_test")
	if err := os.WriteFile(path, []byte("## tui\n\n### theme\n- Charm defaults\n\n### layout\n- Center layout\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Ensure("ws_test"); err != nil {
		t.Fatal(err)
	}
	if index.rebuilds != 2 {
		t.Fatalf("manual edit rebuilds = %d", index.rebuilds)
	}
	matches, err := index.Search("ws_test", Query{})
	if err != nil || len(matches) != 2 {
		t.Fatalf("matches=%#v err=%v", matches, err)
	}
}
