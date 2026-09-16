package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestRegisterLoadsAndUnregisterUnloadsPluginStore(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	manager := newTestManager(t)
	stores := pluginpkg.NewWorkspaceStores(pluginpkg.RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	manager.SetStateHooks(
		func(item Workspace) error {
			_, _, err := stores.Load(item.ID, item.Path)
			return err
		},
		stores.Unload,
		func(item Workspace) error {
			stores.Unload(item.ID)
			_, _, err := stores.Load(item.ID, item.Path)
			return err
		},
	)
	root := t.TempDir()
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	store, ok := stores.Get(item.ID)
	if !ok {
		t.Fatal("registered workspace plugin store missing")
	}
	lockPath := store.Layout().LockPath()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(`{"schema":1,"plugins":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Unregister(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := stores.Get(item.ID); ok {
		t.Fatal("unregistered workspace plugin store still cached")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("unregister removed plugin state: %v", err)
	}
	item, err = manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stores.Get(item.ID); !ok {
		t.Fatal("re-registered workspace plugin store missing")
	}
}

func TestRelocateReloadsWorkspacePluginStoreAtNewRoot(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	manager := newTestManager(t)
	stores := pluginpkg.NewWorkspaceStores(pluginpkg.RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	manager.SetStateHooks(
		func(item Workspace) error {
			_, _, err := stores.Load(item.ID, item.Path)
			return err
		},
		stores.Unload,
		func(item Workspace) error {
			stores.Unload(item.ID)
			_, _, err := stores.Load(item.ID, item.Path)
			return err
		},
	)
	oldRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(oldRoot, 0755); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, ok := stores.Get(item.ID)
	if !ok {
		t.Fatal("registered workspace plugin store missing")
	}
	lockPath := store.Layout().LockPath()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(`{"schema":1,"plugins":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	relocated, err := manager.Relocate(item.ID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := stores.Get(item.ID)
	if !ok {
		t.Fatal("relocated workspace plugin store missing")
	}
	if loaded.Layout().WorkspaceRoot != relocated.Path {
		t.Fatalf("workspace root = %s want %s", loaded.Layout().WorkspaceRoot, relocated.Path)
	}
	if _, err := os.Stat(loaded.Layout().LockPath()); err != nil {
		t.Fatalf("relocated plugin lock missing: %v", err)
	}
}

func TestDeleteStateRemovesWorkspacePluginFiles(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	manager := newTestManager(t)
	root := t.TempDir()
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := pluginpkg.WorkspaceLayout(item.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(layout.LockPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.LockPath(), []byte(`{"schema":1,"plugins":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.DeleteState(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.LockPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("plugin lock survived workspace state deletion")
	}
	if _, err := os.Stat(filepath.Join(item.Path, ".cgm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(".cgm survived workspace state deletion")
	}
}
