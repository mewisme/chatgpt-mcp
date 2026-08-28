package memory

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendWorkspaceMemory(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	path, err := store.Append("ws_test", "use pnpm")
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
	if !strings.Contains(content, "use pnpm") {
		t.Fatalf("content = %q", content)
	}
}
