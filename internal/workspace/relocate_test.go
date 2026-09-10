package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestRelocatePreservesLegacyIDContainersAndState(t *testing.T) {
	storeRoot := t.TempDir()
	store := filepath.Join(storeRoot, "workspaces.json")
	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	if err := os.MkdirAll(filepath.Join(oldRoot, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store)
	oldPlaceholder := filepath.Join(t.TempDir(), "registered")
	if err := os.MkdirAll(oldPlaceholder, 0755); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Register(oldPlaceholder)
	if err != nil {
		t.Fatal(err)
	}
	oldID := item.ID
	container, err := manager.CreateContainer("group")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddWorkspaceToContainer(container.ID, oldID); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(storeRoot, "workspaces", oldID)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(stateDir, "shell"+configformat.ExtensionForRoot(storeRoot))
	stateValue := map[string]any{"workspace_id": oldID, "cwd": filepath.Join(oldPlaceholder, "nested"), "paths": []any{oldPlaceholder, filepath.Join(oldPlaceholder, "file.txt")}}
	data, err := configformat.MarshalPath(statePath, stateValue)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(oldPlaceholder); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(newRoot, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	relocated, err := manager.Relocate(oldID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if relocated.ID == oldID || relocated.Path != canonicalRoot(newRoot) || !containsString(relocated.LegacyIDs, oldID) {
		t.Fatalf("relocated=%#v", relocated)
	}
	if resolved, err := manager.Get(oldID); err != nil || resolved.ID != relocated.ID {
		t.Fatalf("legacy lookup=%#v err=%v", resolved, err)
	}
	updatedContainer, err := manager.GetContainer(container.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updatedContainer.WorkspaceIDs, []string{relocated.ID}) {
		t.Fatalf("container members=%#v", updatedContainer.WorkspaceIDs)
	}
	newStatePath := filepath.Join(storeRoot, "workspaces", relocated.ID, filepath.Base(statePath))
	encoded, err := os.ReadFile(newStatePath)
	if err != nil {
		t.Fatal(err)
	}
	format, err := configformat.Detect(newStatePath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := configformat.DecodeGeneric(format, encoded)
	if err != nil {
		t.Fatal(err)
	}
	stateMap := decoded.(map[string]any)
	if stateMap["workspace_id"] != relocated.ID || stateMap["cwd"] != filepath.Join(canonicalRoot(newRoot), "nested") {
		t.Fatalf("state=%#v", stateMap)
	}
}

func TestRelocateRejectsRegisteredDestination(t *testing.T) {
	manager := newTestManager(t)
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(first.ID, second.Path); err == nil {
		t.Fatal("expected registered destination to be rejected")
	}
}
