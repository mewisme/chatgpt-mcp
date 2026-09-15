package plugin

import (
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
