package workspace

import (
	"path/filepath"
	"testing"
)

func TestUnregisterRemovesOnlyRegistryEntry(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Unregister(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(item.ID); err == nil {
		t.Fatal("workspace still registered")
	}
	if _, err := manager.Register(root); err != nil {
		t.Fatalf("project directory was affected: %v", err)
	}
}
