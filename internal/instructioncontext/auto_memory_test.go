package instructioncontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/memory"
)

func TestLoadAutoMemory(t *testing.T) {
	store := memory.NewStore(t.TempDir())
	path, err := store.Append("ws_test", "use pnpm")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadAutoMemory(store, "ws_test")
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Loaded || snapshot.Bytes != len([]byte(snapshot.Content)) || !strings.Contains(snapshot.Content, "use pnpm") {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if !strings.HasSuffix(path, filepath.Join("ws_test", "MEMORY.md")) {
		t.Fatalf("path = %q", path)
	}
}

func TestLoadAutoMemoryMissingIsEmpty(t *testing.T) {
	store := memory.NewStore(t.TempDir())
	snapshot, err := LoadAutoMemory(store, "ws_missing")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Loaded || snapshot.Content != "" || snapshot.Bytes != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestLoadAutoMemoryRejectsMissingWorkspaceID(t *testing.T) {
	store := memory.NewStore(t.TempDir())
	if _, err := LoadAutoMemory(store, "  "); err == nil {
		t.Fatal("expected missing workspace id to fail")
	}
}

func TestLoadAutoMemoryPropagatesReadErrors(t *testing.T) {
	root := t.TempDir()
	store := memory.NewStore(root)
	path := store.WorkspacePath("ws_test")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAutoMemory(store, "ws_test"); err == nil {
		t.Fatal("expected memory read error")
	}
}
