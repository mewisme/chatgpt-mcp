package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func writeRegistryVersion(t *testing.T, path string, version int, item Workspace) {
	t.Helper()
	data, err := json.MarshalIndent(struct {
		Version    int         `json:"version"`
		Workspaces []Workspace `json:"workspaces"`
	}{Version: version, Workspaces: []Workspace{item}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceIdentityIsStableAcrossManagers(t *testing.T) {
	workspaceRoot := t.TempDir()
	first, err := NewManager(filepath.Join(t.TempDir(), "workspaces.json")).Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewManager(filepath.Join(t.TempDir(), "workspaces.json")).Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Path != second.Path || !strings.HasPrefix(first.ID, "ws_") {
		t.Fatalf("workspace identity is not stable: %#v %#v", first, second)
	}
	identity, err := workspacestate.New(workspaceRoot).LoadIdentity()
	if err != nil || identity.ID != first.ID {
		t.Fatalf("local identity=%#v err=%v", identity, err)
	}
}

func TestWorkspaceRegistryV2MigratesStateLocalAndPreservesID(t *testing.T) {
	configRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	oldID := instanceScopedWorkspaceID("inst_33333333333333333333333333333333", workspaceRoot)
	legacyAlias := workspaceID(workspaceRoot)
	registryPath := filepath.Join(configRoot, "workspaces.json")
	writeRegistryVersion(t, registryPath, 2, Workspace{ID: oldID, Path: workspaceRoot, LegacyIDs: []string{legacyAlias}})
	oldState := filepath.Join(configRoot, "workspaces", oldID)
	if err := os.MkdirAll(filepath.Join(oldState, "checkpoints", "data"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]string{
		filepath.Join(oldState, "marker.txt"):                           "v2",
		filepath.Join(oldState, "shell.json"):                           `{"workspace_id":"` + oldID + `","cwd":"` + workspaceRoot + `"}`,
		filepath.Join(oldState, "checkpoints", "data", "manifest.json"): `{"workspace_id":"` + oldID + `"}`,
	} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}

	manager := NewManager(registryPath)
	items, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != oldID || len(items[0].LegacyIDs) != 1 || items[0].LegacyIDs[0] != legacyAlias {
		t.Fatalf("migrated workspace=%#v", items)
	}
	if resolved, err := manager.Get(legacyAlias); err != nil || resolved.ID != oldID {
		t.Fatalf("legacy alias resolved=%#v err=%v", resolved, err)
	}
	if _, err := os.Stat(oldState); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy global state remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "workspaces")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty leftover workspaces dir remains: %v", err)
	}
	local := workspacestate.New(workspaceRoot)
	for path, want := range map[string]string{
		filepath.Join(local.StateRoot(), "marker.txt"):                 "v2",
		filepath.Join(local.StateRoot(), "shell.json"):                 `{"workspace_id":"` + oldID + `","cwd":"` + workspaceRoot + `"}`,
		filepath.Join(local.CheckpointRoot(), "data", "manifest.json"): `{"workspace_id":"` + oldID + `"}`,
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("path=%s data=%q err=%v", path, data, err)
		}
	}
	var stored storeFile
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Version != storeVersion || len(stored.Workspaces) != 1 || stored.Workspaces[0].ID != oldID || len(stored.Workspaces[0].AllowDirs) != 0 || len(stored.Workspaces[0].LegacyIDs) != 0 {
		t.Fatalf("stored registry=%#v", stored)
	}
}

func TestWorkspaceRegistryV1UpgradeKeepsID(t *testing.T) {
	configRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	id := workspaceID(workspaceRoot)
	registryPath := filepath.Join(configRoot, "workspaces.json")
	writeRegistryVersion(t, registryPath, 1, Workspace{ID: id, Path: workspaceRoot})
	item, err := NewManager(registryPath).Get(id)
	if err != nil || item.ID != id {
		t.Fatalf("workspace=%#v err=%v", item, err)
	}
	identity, err := workspacestate.New(workspaceRoot).LoadIdentity()
	if err != nil || identity.ID != id {
		t.Fatalf("identity=%#v err=%v", identity, err)
	}
}

func TestWorkspaceConfigSurvivesUnregisterAndReregister(t *testing.T) {
	registryPath := filepath.Join(t.TempDir(), "workspaces.json")
	workspaceRoot := t.TempDir()
	allowed := t.TempDir()
	manager := NewManager(registryPath)
	item, err := manager.Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	item, err = manager.AddAllowDir(item.ID, allowed)
	if err != nil || len(item.AllowDirs) != 1 {
		t.Fatalf("add allow dir=%#v err=%v", item, err)
	}
	if err := manager.Unregister(item.ID); err != nil {
		t.Fatal(err)
	}
	reregistered, err := manager.Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if reregistered.ID != item.ID || len(reregistered.AllowDirs) != 1 || reregistered.AllowDirs[0] != canonicalRoot(allowed) {
		t.Fatalf("reregistered=%#v want id=%s allow=%s", reregistered, item.ID, canonicalRoot(allowed))
	}
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "allow_dirs") || strings.Contains(string(data), "legacy_ids") {
		t.Fatalf("global registry contains workspace-owned config: %s", data)
	}
}

func TestWorkspaceMigrationKeepsLocalFilesOnConflict(t *testing.T) {
	configRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	id := instanceScopedWorkspaceID("inst_55555555555555555555555555555555", workspaceRoot)
	registryPath := filepath.Join(configRoot, "workspaces.json")
	writeRegistryVersion(t, registryPath, 2, Workspace{ID: id, Path: workspaceRoot})
	legacyState := filepath.Join(configRoot, "workspaces", id)
	if err := os.MkdirAll(legacyState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyState, "marker.txt"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	local := workspacestate.New(workspaceRoot)
	if _, _, err := local.EnsureIdentity(id); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(local.StateRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local.StateRoot(), "marker.txt"), []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := NewManager(registryPath).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Available() || items[0].ID != id {
		t.Fatalf("expected leftover global state to migrate, got %#v", items)
	}
	data, err := os.ReadFile(filepath.Join(local.StateRoot(), "marker.txt"))
	if err != nil || string(data) != "different" {
		t.Fatalf("local file overwritten: %q err=%v", data, err)
	}
	if _, err := os.Stat(legacyState); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state remains: %v", err)
	}
}
