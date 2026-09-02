package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveContextUsesRegisteredRoot(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := manager.ResolveContext(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Workspace.ID != item.ID || filepath.Clean(ctx.Root) != filepath.Clean(item.Path) {
		t.Fatalf("context = %#v, want workspace %s root %s", ctx, item.ID, item.Path)
	}
}

func TestResolveWorkspacePathUsesRegisteredRoot(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "nested", "file.txt")
	if err := os.MkdirAll(filepath.Dir(child), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, resolved, err := manager.ResolveWorkspacePath(item.ID, filepath.Join("nested", "file.txt"), true)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Workspace.ID != item.ID || filepath.Clean(resolved) != filepath.Clean(child) {
		t.Fatalf("context/resolved = %#v %s, want %s %s", ctx, resolved, item.ID, child)
	}
}
