package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestStoreImmutableInstallAndRollback(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, version := range []string{"1.0.0", "1.1.0"} {
		manifest := testManifest("bash", version, "shell/bash")
		payload := testPayload(t, "bash")
		if _, err := store.Install(manifest, payload); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Install(testManifest("bash", "1.0.0", "shell/bash"), testPayload(t, "bash")); !errors.Is(err, ErrVersionInstalled) {
		t.Fatalf("duplicate install error = %v", err)
	}
	if err := store.Activate("bash", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.1.0", trust); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["bash"].Version != "1.0.0" {
		t.Fatalf("rollback version = %q", lock.Plugins["bash"].Version)
	}
	versions, err := store.InstalledVersions("bash")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("installed versions = %#v", versions)
	}
	if err := store.RemoveVersion("bash", "1.0.0"); err == nil {
		t.Fatal("active rollback version was removed")
	}
}

func TestStoreRejectsUntrustedActivation(t *testing.T) {
	store := testStore(t)
	if _, err := store.Install(testManifest("bash", "1.0.0", "shell/bash"), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme"}); err == nil {
		t.Fatal("untrusted plugin activated")
	}
}

func TestStoreRejectsPayloadSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require additional privileges")
	}
	store := testStore(t)
	payload := testPayload(t, "bash")
	if err := os.Symlink("/tmp", filepath.Join(payload, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.Install(testManifest("bash", "1.0.0", "shell/bash"), payload); err == nil {
		t.Fatal("payload symlink accepted")
	}
}

func TestStoreEnableRevalidatesCoreCompatibility(t *testing.T) {
	store := testStore(t)
	store.runtime.CoreVersion = "10.0.0"
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	manifest.Requires.ChatGPTMCP = ">=9.0.0"
	if _, err := store.Install(manifest, testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetEnabled("bash", false); err != nil {
		t.Fatal(err)
	}
	store.runtime.CoreVersion = "0.2.24"
	if err := store.SetEnabled("bash", true); err == nil {
		t.Fatal("incompatible plugin re-enabled")
	}
}

func TestStoreActivateWithStatePersistsDisabledLockAndDesiredAtomically(t *testing.T) {
	store := testStore(t)
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	if _, err := store.Install(manifest, testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if err := store.ActivateWithState("bash", "1.0.0", trust, false); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := lock.Plugins["bash"]
	if !ok || entry.Enabled || entry.Version != "1.0.0" {
		t.Fatalf("lock entry = %#v", entry)
	}
	config, err := LoadConfig(store.layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	desired, ok := config.Desired["bash"]
	if !ok || desired.Enabled || desired.Version != "1.0.0" || desired.Registry != "official" {
		t.Fatalf("desired entry = %#v", desired)
	}
}

func TestStoreDisableIfEnabledIsIdempotent(t *testing.T) {
	store := testStore(t)
	if _, err := store.Install(testManifest("bash", "1.0.0", "shell/bash"), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	changed, err := store.DisableIfEnabled("bash")
	if err != nil || !changed {
		t.Fatalf("first disable changed=%t err=%v", changed, err)
	}
	changed, err = store.DisableIfEnabled("bash")
	if err != nil || changed {
		t.Fatalf("second disable changed=%t err=%v", changed, err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["bash"].Enabled {
		t.Fatal("plugin remained enabled")
	}
	config, err := LoadConfig(store.layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if desired, ok := config.Desired["bash"]; !ok || desired.Enabled || desired.Version != "1.0.0" || desired.Registry != "official" {
		t.Fatalf("desired Bash state = %#v ok=%t", desired, ok)
	}
	changed, err = store.DisableIfEnabled("missing")
	if err != nil || changed {
		t.Fatalf("missing plugin disable changed=%t err=%v", changed, err)
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testWorkspaceStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	layout, err := WorkspaceLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testPayload(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", name), []byte("payload"), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLeftoverGlobalPluginsRemainGlobal(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	layout := DefaultLayout()
	if layout.EffectiveScope() != ScopeGlobal || filepath.Base(layout.ConfigPath()) != "plugins.json" || filepath.Base(layout.LockPath()) != "plugins.lock.json" {
		t.Fatalf("leftover layout = %#v", layout)
	}
	config := NewConfig()
	if err := config.SetDesired("bash", OfficialRegistryName, "1.0.0", true); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	loaded, err := layout.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	desired, ok := loaded.Desired["bash"]
	if !ok || desired.Version != "1.0.0" || !desired.Enabled {
		t.Fatalf("leftover desired = %#v", loaded.Desired)
	}
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	if store.Layout().EffectiveScope() != ScopeGlobal {
		t.Fatalf("store layout = %#v", store.Layout())
	}
}

func TestStoreRejectsDisallowedInstallScope(t *testing.T) {
	layout, err := WorkspaceLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Install(testManifest("bash", "1.0.0", "shell/bash"), testPayload(t, "bash")); !errors.Is(err, ErrScopeNotAllowed) {
		t.Fatalf("schema 1 workspace install error = %v", err)
	}
	if _, err := testStore(t).Install(testScopedManifest("bash", "1.0.0", "shell/bash", ScopeWorkspace), testPayload(t, "bash")); !errors.Is(err, ErrScopeNotAllowed) {
		t.Fatalf("workspace-only global install error = %v", err)
	}
}

func TestWorkspaceStoreOmitsCompiledBuiltins(t *testing.T) {
	previous := compiledBuiltinClone()
	t.Cleanup(func() { SetCompiledBuiltins(previous) })
	SetCompiledBuiltins(BuiltinRegistry{testBuiltin("ponytail", "tool/ponytail")})
	layout, err := WorkspaceLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Builtins) != 0 {
		t.Fatalf("workspace builtins = %#v", store.Builtins)
	}
}
