package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
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
	if _, err := os.Stat(workspacestate.New(root).IdentityPath()); err != nil {
		t.Fatalf("unregister removed local state: %v", err)
	}
	if _, err := manager.Register(root); err != nil {
		t.Fatalf("project directory was affected: %v", err)
	}
}

func TestDeleteStateUnregistersAndRemovesLocalState(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "project.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.DeleteState(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(item.ID); err == nil {
		t.Fatal("workspace still registered")
	}
	if _, err := os.Stat(workspacestate.New(root).Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local state remains: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "keep" {
		t.Fatalf("project files changed: data=%q err=%v", data, err)
	}
	recreated, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if recreated.ID == item.ID {
		t.Fatalf("deleted identity was reused: %s", recreated.ID)
	}
}

func TestDeleteStateRemovesUnregisteredCopiedIdentity(t *testing.T) {
	source := t.TempDir()
	copyRoot := t.TempDir()
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(source)
	if err != nil {
		t.Fatal(err)
	}
	copyTree(t, workspacestate.New(source).Root(), workspacestate.New(copyRoot).Root())
	if _, err := manager.Register(copyRoot); err == nil {
		t.Fatal("copied identity was accepted")
	}
	if _, err := manager.DeleteState(copyRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspacestate.New(copyRoot).Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copied local state remains: %v", err)
	}
	copied, err := manager.Register(copyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID == item.ID {
		t.Fatalf("copied workspace reused identity %s", copied.ID)
	}
	if _, err := manager.Get(item.ID); err != nil {
		t.Fatalf("original workspace was affected: %v", err)
	}
}

func TestDeleteStateRefusesWhileAnotherRuntimeHoldsTheLock(t *testing.T) {
	store := filepath.Join(t.TempDir(), "workspaces.json")
	root := t.TempDir()
	first := NewManager(store)
	item, err := first.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Activate(); err != nil {
		t.Fatal(err)
	}
	defer first.Deactivate()
	if _, err := NewManager(store).DeleteState(item.ID); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("delete-state error=%v", err)
	}
	if _, err := first.Get(item.ID); err != nil {
		t.Fatalf("active workspace was deleted: %v", err)
	}
	if _, err := os.Stat(workspacestate.New(root).IdentityPath()); err != nil {
		t.Fatalf("active local state was deleted: %v", err)
	}
}

func TestDeleteStateSucceedsWhenLocalStateAlreadyMissing(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	root := t.TempDir()
	manager := NewManager(store)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(workspacestate.New(root).Root()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(store).DeleteState(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(store).Get(item.ID); err == nil {
		t.Fatal("workspace still registered")
	}
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	if err := os.MkdirAll(destination, 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			copyTree(t, from, to)
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
