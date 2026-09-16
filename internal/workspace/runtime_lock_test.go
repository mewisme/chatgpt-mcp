package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func TestWorkspaceRuntimeLockRejectsSecondActiveManager(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
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
	data, err := os.ReadFile(workspacestate.New(root).RuntimeLockPath())
	if err != nil {
		t.Fatal(err)
	}
	var metadata runtimeLockMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.PID <= 0 || metadata.WorkspaceID != item.ID || metadata.StartedAt.IsZero() {
		t.Fatalf("lock metadata=%#v", metadata)
	}
	second := NewManager(store)
	if err := second.Activate(); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("second activation error=%v", err)
	}
	if err := first.Deactivate(); err != nil {
		t.Fatal(err)
	}
	if err := second.Activate(); err != nil {
		t.Fatalf("activation after release failed: %v", err)
	}
	if err := second.Deactivate(); err != nil {
		t.Fatal(err)
	}
}

func TestActivateKeepsUnavailableSiblingListed(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	manager := NewManager(store)
	healthy, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(missing.Path); err != nil {
		t.Fatal(err)
	}
	active := NewManager(store)
	if err := active.Activate(); err != nil {
		t.Fatal(err)
	}
	defer active.Deactivate()
	listed, err := active.List()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Workspace{}
	for _, item := range listed {
		byID[item.ID] = item
	}
	if !byID[healthy.ID].Available() {
		t.Fatalf("healthy workspace unavailable: %#v", byID[healthy.ID])
	}
	if byID[missing.ID].Available() {
		t.Fatalf("missing workspace available: %#v", byID[missing.ID])
	}
	if _, err := os.Stat(workspacestate.New(healthy.Path).RuntimeLockPath()); err != nil {
		t.Fatal(err)
	}
	got, err := active.Get(healthy.ID)
	if err != nil || !got.Available() {
		t.Fatalf("healthy get=%#v err=%v", got, err)
	}
}

func TestActiveWorkspaceStateLossFailsClosed(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	root := t.TempDir()
	manager := NewManager(store)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer manager.Deactivate()
	local := workspacestate.New(root)
	if err := os.Remove(local.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(item.ID); !errors.Is(err, ErrStateLost) {
		t.Fatalf("get after state loss error=%v", err)
	}
	listed, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Available() {
		t.Fatalf("list after state loss=%#v", listed)
	}
	if _, err := manager.AddAllowDir(item.ID, t.TempDir()); !errors.Is(err, ErrStateLost) {
		t.Fatalf("mutation after state loss error=%v", err)
	}
	if _, err := os.Stat(local.ConfigPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing config was recreated: %v", err)
	}
}

func TestActiveRegisterDoesNotRecreateMissingIdentity(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	root := t.TempDir()
	manager := NewManager(store)
	if _, err := manager.Register(root); err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer manager.Deactivate()
	identityPath := workspacestate.New(root).IdentityPath()
	if err := os.Remove(identityPath); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(root); !errors.Is(err, ErrStateLost) {
		t.Fatalf("register after identity loss error=%v", err)
	}
	if _, err := os.Stat(identityPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing identity was recreated: %v", err)
	}
}

func TestActiveRegisterLocksBeforeLoadingWorkspaceConfig(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	owner := NewManager(filepath.Join(t.TempDir(), "owner.json"))
	if _, err := owner.Register(root); err != nil {
		t.Fatal(err)
	}
	if err := owner.Activate(); err != nil {
		t.Fatal(err)
	}
	defer owner.Deactivate()
	contender := NewManager(filepath.Join(t.TempDir(), "contender.json"))
	if err := contender.Activate(); err != nil {
		t.Fatal(err)
	}
	defer contender.Deactivate()
	if _, err := contender.Register(root); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("active register error=%v", err)
	}
}

func TestStandaloneRegisterRejectsWorkspaceOwnedByActiveRuntime(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	owner := NewManager(filepath.Join(t.TempDir(), "owner.json"))
	if _, err := owner.Register(root); err != nil {
		t.Fatal(err)
	}
	if err := owner.Activate(); err != nil {
		t.Fatal(err)
	}
	defer owner.Deactivate()
	contender := NewManager(filepath.Join(t.TempDir(), "contender.json"))
	if _, err := contender.Register(root); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("standalone register error=%v", err)
	}
	if contender.Active() {
		t.Fatal("failed standalone registration left transient runtime active")
	}
}

func TestStandaloneAllowDirMutationRejectsWorkspaceOwnedByActiveRuntime(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	owner := NewManager(store)
	item, err := owner.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Activate(); err != nil {
		t.Fatal(err)
	}
	defer owner.Deactivate()
	allowed := t.TempDir()
	if _, err := NewManager(store).AddAllowDir(item.ID, allowed); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("standalone allow-dir mutation error=%v", err)
	}
	config, err := workspacestate.New(item.Path).LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AllowDirs) != 0 {
		t.Fatalf("allow dirs changed despite rejected mutation: %#v", config.AllowDirs)
	}
}

func TestStandaloneUnregisterRejectsWorkspaceOwnedByActiveRuntime(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	owner := NewManager(store)
	item, err := owner.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Activate(); err != nil {
		t.Fatal(err)
	}
	defer owner.Deactivate()
	if err := NewManager(store).Unregister(item.ID); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("standalone unregister error=%v", err)
	}
	if got, err := owner.Get(item.ID); err != nil || got.ID != item.ID {
		t.Fatalf("workspace disappeared after rejected unregister: %#v err=%v", got, err)
	}
}

func TestActivateMigratesLegacyRegistryAfterAcquiringLock(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	configRoot := t.TempDir()
	root := t.TempDir()
	id := instanceScopedWorkspaceID("inst_legacy_runtime_lock", root)
	store := filepath.Join(configRoot, "workspaces.json")
	writeRegistryVersion(t, store, 2, Workspace{ID: id, Path: root})
	legacyState := filepath.Join(configRoot, "workspaces", id)
	if err := os.MkdirAll(legacyState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyState, "marker.txt"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store)
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer manager.Deactivate()
	if _, err := os.Stat(workspacestate.New(root).RuntimeLockPath()); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(workspacestate.New(root).StatePath("marker.txt")); err != nil || string(data) != "legacy" {
		t.Fatalf("migrated state=%q err=%v", data, err)
	}
}

func TestActiveReloadAcquiresLockForNewWorkspace(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	manager := NewManager(store)
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer manager.Deactivate()
	secondRoot := t.TempDir()
	secondIdentity, _, err := workspacestate.New(secondRoot).EnsureIdentity("")
	if err != nil {
		t.Fatal(err)
	}
	if err := workspacestate.New(secondRoot).SaveConfig(workspacestate.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	writeCurrentRegistryForRuntimeLockTest(t, store, first, Workspace{ID: secondIdentity.ID, Path: secondRoot})
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(secondIdentity.ID); err != nil {
		t.Fatal(err)
	}
	contenderStore := filepath.Join(t.TempDir(), "contender.json")
	writeCurrentRegistryForRuntimeLockTest(t, contenderStore, Workspace{ID: secondIdentity.ID, Path: secondRoot})
	if err := NewManager(contenderStore).Activate(); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("contender activation error=%v", err)
	}
}

func TestActiveReloadRejectsBusyNewWorkspaceBeforeLoadingState(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	manager := NewManager(store)
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer manager.Deactivate()
	blockedStore := filepath.Join(t.TempDir(), "blocked.json")
	blockedManager := NewManager(blockedStore)
	blocked, err := blockedManager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := blockedManager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer blockedManager.Deactivate()
	writeCurrentRegistryForRuntimeLockTest(t, store, first, blocked)
	if err := os.Remove(workspacestate.New(blocked.Path).ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("reload error=%v", err)
	}
	if _, err := os.Stat(workspacestate.New(blocked.Path).ConfigPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("busy workspace config was touched before lock acquisition: %v", err)
	}
	if got, err := manager.Get(first.ID); err != nil || got.ID != first.ID {
		t.Fatalf("existing workspace changed after failed reload: %#v err=%v", got, err)
	}
}

func writeCurrentRegistryForRuntimeLockTest(t *testing.T, path string, items ...Workspace) {
	t.Helper()
	stored := make([]storedWorkspace, 0, len(items))
	for _, item := range items {
		stored = append(stored, storedWorkspace{ID: item.ID, Path: item.Path})
	}
	data, err := configformat.MarshalPath(path, storeFile{Version: storeVersion, Workspaces: stored})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
