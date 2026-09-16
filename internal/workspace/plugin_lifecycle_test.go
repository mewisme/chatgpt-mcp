package workspace

import (
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
