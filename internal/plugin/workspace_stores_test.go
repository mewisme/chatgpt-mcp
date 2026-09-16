package plugin

import (
	"os"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestWorkspaceStoresLoadUnloadPreservesLock(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	stores := NewWorkspaceStores(RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	root := t.TempDir()
	store, _, err := stores.Load("ws_a", root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(testScopedManifest("bash", "1.0.0", "shell/bash", ScopeWorkspace), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	lockPath := store.Layout().LockPath()
	stores.Unload("ws_a")
	if _, ok := stores.Get("ws_a"); ok {
		t.Fatal("unloaded workspace store still cached")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("unload removed plugin state: %v", err)
	}
	loaded, _, err := stores.Load("ws_a", root)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(loaded.Layout().LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := lock.Plugins["bash"]; !ok || !entry.Enabled {
		t.Fatalf("reloaded lock = %#v", lock.Plugins)
	}
}

func TestWorkspaceStoresIsolateWorkspaces(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	stores := NewWorkspaceStores(RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	a, _, err := stores.Load("ws_a", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := stores.Load("ws_b", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Install(testScopedManifest("bash", "1.0.0", "shell/bash", ScopeWorkspace), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := a.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.Layout().LockPath()); err == nil {
		t.Fatal("workspace B lock created by workspace A install")
	}
	if sameCleanPath(a.Layout().ConfigRoot, b.Layout().ConfigRoot) {
		t.Fatal("workspace plugin stores share config root")
	}
}

func TestWorkspaceStoresSetGlobalPeer(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	global := testStore(t)
	stores := NewWorkspaceStores(RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	workspace, _, err := stores.Load("ws_a", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stores.SetGlobalPeer(global)
	if len(global.Peers()) != 1 || global.Peers()[0] != workspace {
		t.Fatalf("global peers = %#v", global.Peers())
	}
	if len(workspace.Peers()) != 1 || workspace.Peers()[0] != global {
		t.Fatalf("workspace peers = %#v", workspace.Peers())
	}
	stores.Unload("ws_a")
	stores.SetGlobalPeer(global)
	if len(global.Peers()) != 0 {
		t.Fatalf("unloaded peer remained: %#v", global.Peers())
	}
}
