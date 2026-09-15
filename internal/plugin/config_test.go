package plugin

import (
	"os"
	"path/filepath"
	"testing"
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
