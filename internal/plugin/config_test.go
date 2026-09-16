package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestConfigRegistryLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	config := NewConfig()
	trust := SigstoreIdentity{Issuer: "https://token.actions.githubusercontent.com", Repository: "example/plugins"}
	if err := config.AddRegistry("community", "https://plugins.example.test/releases", false, trust); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.AllRegistries()) != 2 {
		t.Fatalf("registries = %#v", loaded.AllRegistries())
	}
	if err := loaded.RemoveRegistry("official"); err == nil {
		t.Fatal("official registry removed")
	}
	if err := loaded.RemoveRegistry("community"); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRegistryRequiresPinnedTrust(t *testing.T) {
	config := NewConfig()
	if err := config.AddRegistry("community", "https://plugins.example.test/releases", false, SigstoreIdentity{}); err == nil {
		t.Fatal("third-party registry without pinned trust accepted")
	}
}

func TestConfigDesiredStateIsPortableAndProtectsRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	config := NewConfig()
	trust := SigstoreIdentity{Issuer: "https://token.actions.githubusercontent.com", Repository: "example/plugins"}
	if err := config.AddRegistry("community", "https://plugins.example.test/releases", false, trust); err != nil {
		t.Fatal(err)
	}
	if err := config.SetDesired("formatter", "community", "1.2.3", false); err != nil {
		t.Fatal(err)
	}
	if err := config.RemoveRegistry("community"); err == nil {
		t.Fatal("registry referenced by desired plugin was removed")
	}
	if err := WriteConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	desired := loaded.Desired["formatter"]
	if desired.Registry != "community" || desired.Version != "1.2.3" || desired.Enabled {
		t.Fatalf("desired state = %#v", desired)
	}
}

func TestMutateConfigSerializesAcrossStoreInstances(t *testing.T) {
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	trust := SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: "example/plugins"}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- MutateConfig(layout, func(config *Config) error {
			close(entered)
			<-release
			return config.AddRegistry("one", "https://one.example.test", false, trust)
		})
	}()
	<-entered
	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- MutateConfig(layout, func(config *Config) error {
			close(secondEntered)
			return config.AddRegistry("two", "https://two.example.test", false, trust)
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second plugin mutation entered while first mutation lock was held")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := config.Registries["one"]; !ok {
		t.Fatal("first serialized registry mutation was lost")
	}
	if _, ok := config.Registries["two"]; !ok {
		t.Fatal("second serialized registry mutation was lost")
	}
}

func TestConfigLegacyFileInitializesDesiredState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"registries":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Desired == nil || len(config.Desired) != 0 {
		t.Fatalf("legacy desired state = %#v", config.Desired)
	}
}

func isolatedWorkspaceLayout(t *testing.T) Layout {
	t.Helper()
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	layout, err := WorkspaceLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func TestWorkspaceDesiredOmitsRegistriesAndDoesNotLeakGlobal(t *testing.T) {
	layout := isolatedWorkspaceLayout(t)
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(testScopedManifest("bash", "1.0.0", "shell/bash", ScopeWorkspace), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"registries"`) {
		t.Fatalf("workspace desired included registries: %s", data)
	}
	loaded, err := layout.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	desired, ok := loaded.Desired["bash"]
	if !ok || desired.Registry != "official" || desired.Version != "1.0.0" {
		t.Fatalf("workspace desired = %#v", loaded.Desired)
	}
	global, err := LoadConfig(DefaultLayout().ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := global.Desired["bash"]; leaked {
		t.Fatalf("workspace desired leaked into global config: %#v", global.Desired)
	}
}

func TestWorkspaceDesiredRejectsEmbeddedRegistries(t *testing.T) {
	layout := isolatedWorkspaceLayout(t)
	if err := os.MkdirAll(filepath.Dir(layout.ConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ConfigPath(), []byte(`{"schema":1,"registries":{},"desired":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := layout.LoadConfig(); err == nil {
		t.Fatal("workspace desired accepted registries")
	}
}

func TestWorkspaceMutateConfigRejectsRegistryChanges(t *testing.T) {
	layout := isolatedWorkspaceLayout(t)
	trust := SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: "example/plugins"}
	if err := MutateConfig(layout, func(config *Config) error {
		return config.AddRegistry("community", "https://plugins.example.test/releases", false, trust)
	}); err == nil {
		t.Fatal("workspace layout accepted registry mutation")
	}
}
