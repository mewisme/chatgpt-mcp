package plugin

import (
	"errors"
	"testing"
)

func TestCapabilityResolver(t *testing.T) {
	store := testStore(t)
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	if _, err := store.Install(manifest, testPayload(t)); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.Resolve("shell/bash")
	if err != nil {
		t.Fatal(err)
	}
	if provider.PluginID != "bash" || provider.Version != "1.0.0" {
		t.Fatalf("provider = %#v", provider)
	}
	if _, err := resolver.Resolve("formatter/json"); !errors.Is(err, ErrCapabilityNotFound) {
		t.Fatalf("missing capability error = %v", err)
	}
}

func TestCapabilityResolverConflictAndDisabledProvider(t *testing.T) {
	store := testStore(t)
	for _, id := range []PluginID{"bash", "other-bash"} {
		manifest := testManifest(string(id), "1.0.0", "shell/bash")
		if _, err := store.Install(manifest, testPayload(t)); err != nil {
			t.Fatal(err)
		}
	}
	lock := NewLockFile()
	for _, id := range []PluginID{"bash", "other-bash"} {
		manifest := testManifest(string(id), "1.0.0", "shell/bash")
		lock.Plugins[id] = testLockEntry(t, store, manifest, "1.0.0")
	}
	if err := WriteLock(store.layout.LockPath(), lock); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve("shell/bash"); err == nil {
		t.Fatal("capability conflict silently resolved")
	} else {
		var conflict CapabilityConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("conflict error = %T %v", err, err)
		}
	}
	entry := lock.Plugins["other-bash"]
	entry.Enabled = false
	lock.Plugins["other-bash"] = entry
	if err := WriteLock(store.layout.LockPath(), lock); err != nil {
		t.Fatal(err)
	}
	resolver, err = NewResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.Resolve("shell/bash")
	if err != nil || provider.PluginID != "bash" {
		t.Fatalf("provider = %#v, %v", provider, err)
	}
}
