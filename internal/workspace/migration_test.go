package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestInstanceIdentity(t *testing.T, root, id string) {
	t.Helper()
	path := filepath.Join(root, "state", "instance.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"version": 1, "identity": map[string]any{"id": id, "name": "test", "created_at": time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyRegistry(t *testing.T, path string, item Workspace) {
	t.Helper()
	data, err := json.MarshalIndent(struct {
		Version    int         `json:"version"`
		Workspaces []Workspace `json:"workspaces"`
	}{Version: 1, Workspaces: []Workspace{item}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceIDsAreScopedByInstance(t *testing.T) {
	workspaceRoot := t.TempDir()
	rootA, rootB := t.TempDir(), t.TempDir()
	writeTestInstanceIdentity(t, rootA, "inst_11111111111111111111111111111111")
	writeTestInstanceIdentity(t, rootB, "inst_22222222222222222222222222222222")
	first, err := NewManager(filepath.Join(rootA, "workspaces.json")).Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewManager(filepath.Join(rootB, "workspaces.json")).Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("workspace ids collide across instances: %s", first.ID)
	}
}

func TestWorkspaceRegistryMigratesLegacyIDsAndState(t *testing.T) {
	configRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	instanceID := "inst_33333333333333333333333333333333"
	writeTestInstanceIdentity(t, configRoot, instanceID)
	legacyID := legacyWorkspaceID(workspaceRoot)
	registryPath := filepath.Join(configRoot, "workspaces.json")
	writeLegacyRegistry(t, registryPath, Workspace{ID: legacyID, Path: workspaceRoot})
	legacyState := filepath.Join(configRoot, "workspaces", legacyID)
	if err := os.MkdirAll(legacyState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyState, "marker.txt"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyState, "shell.json"), []byte(`{"workspace_id":"`+legacyID+`","cwd":"`+workspaceRoot+`","started_at":"x","updated_at":"x","recent_commands":["pwd"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(legacyState, "checkpoints", "cp_test")
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.json"), []byte(`{"version":1,"id":"cp_test","workspace_id":"`+legacyID+`","workspace_root":"`+workspaceRoot+`","created_at":"x","tool":"edit_file","summary":"test","files":[]}`), 0600); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(registryPath)
	items, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("workspaces = %#v", items)
	}
	canonicalID := workspaceID(instanceID, workspaceRoot)
	if items[0].ID != canonicalID || len(items[0].LegacyIDs) != 1 || items[0].LegacyIDs[0] != legacyID {
		t.Fatalf("migrated workspace = %#v", items[0])
	}
	resolved, err := manager.Get(legacyID)
	if err != nil || resolved.ID != canonicalID {
		t.Fatalf("legacy alias resolved to %#v err=%v", resolved, err)
	}
	if id, err := manager.CanonicalID(legacyID); err != nil || id != canonicalID {
		t.Fatalf("canonical id = %q err=%v", id, err)
	}
	if _, err := os.Stat(legacyState); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state still exists: %v", err)
	}
	marker, err := os.ReadFile(filepath.Join(configRoot, "workspaces", canonicalID, "marker.txt"))
	if err != nil || string(marker) != "legacy" {
		t.Fatalf("migrated state marker = %q err=%v", marker, err)
	}
	for _, path := range []string{
		filepath.Join(configRoot, "workspaces", canonicalID, "shell.json"),
		filepath.Join(configRoot, "workspaces", canonicalID, "checkpoints", "cp_test", "manifest.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		if object["workspace_id"] != canonicalID {
			t.Fatalf("state workspace id in %s = %#v", path, object["workspace_id"])
		}
	}

	reloaded := NewManager(registryPath)
	resolved, err = reloaded.Get(legacyID)
	if err != nil || resolved.ID != canonicalID {
		t.Fatalf("persisted legacy alias resolved to %#v err=%v", resolved, err)
	}
	var stored storeFile
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Version != storeVersion || len(stored.Workspaces) != 1 || stored.Workspaces[0].ID != canonicalID {
		t.Fatalf("stored registry = %#v", stored)
	}
}

func TestLegacyAliasSupportsWorkspaceMutations(t *testing.T) {
	configRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	allowed := t.TempDir()
	instanceID := "inst_44444444444444444444444444444444"
	writeTestInstanceIdentity(t, configRoot, instanceID)
	legacyID := legacyWorkspaceID(workspaceRoot)
	registryPath := filepath.Join(configRoot, "workspaces.json")
	writeLegacyRegistry(t, registryPath, Workspace{ID: legacyID, Path: workspaceRoot})
	manager := NewManager(registryPath)
	item, err := manager.AddAllowDir(legacyID, allowed)
	if err != nil || len(item.AllowDirs) != 1 {
		t.Fatalf("add allow dir = %#v err=%v", item, err)
	}
	if _, err := manager.RemoveAllowDir(legacyID, allowed); err != nil {
		t.Fatal(err)
	}
	if err := manager.Unregister(legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(legacyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy alias remained after unregister: %v", err)
	}
}

func TestWorkspaceMigrationRejectsConflictingStateDirectories(t *testing.T) {
	configRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	instanceID := "inst_55555555555555555555555555555555"
	writeTestInstanceIdentity(t, configRoot, instanceID)
	legacyID := legacyWorkspaceID(workspaceRoot)
	canonicalID := workspaceID(instanceID, workspaceRoot)
	registryPath := filepath.Join(configRoot, "workspaces.json")
	writeLegacyRegistry(t, registryPath, Workspace{ID: legacyID, Path: workspaceRoot})
	for _, id := range []string{legacyID, canonicalID} {
		if err := os.MkdirAll(filepath.Join(configRoot, "workspaces", id), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewManager(registryPath).List(); err == nil {
		t.Fatal("expected conflicting workspace state migration to fail")
	}
}
