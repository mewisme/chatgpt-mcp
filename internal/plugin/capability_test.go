package plugin

import (
	"errors"
	"testing"
)

func TestResolverDeterministicConflictAndDisable(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, id := range []string{"bash-a", "bash-b"} {
		manifest := testManifest(id, "1.0.0", "shell/bash")
		if _, err := store.Install(manifest, testPayload(t, id)); err != nil {
			t.Fatal(err)
		}
		if err := store.Activate(PluginID(id), "1.0.0", trust); err != nil {
			t.Fatal(err)
		}
	}
	resolver, err := NewResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	providers := resolver.Providers("shell/bash")
	if len(providers) != 2 || providers[0].PluginID != "bash-a" || providers[1].PluginID != "bash-b" {
		t.Fatalf("providers = %#v", providers)
	}
	if _, err := resolver.Resolve("shell/bash"); err == nil {
		t.Fatal("conflicting singleton capability resolved silently")
	} else {
		var conflict CapabilityConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("resolve error = %T %v", err, err)
		}
	}
	if err := store.SetEnabled("bash-b", false); err != nil {
		t.Fatal(err)
	}
	resolver, err = NewResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.Resolve("shell/bash")
	if err != nil {
		t.Fatal(err)
	}
	if provider.PluginID != "bash-a" {
		t.Fatalf("provider = %q", provider.PluginID)
	}
}

func TestResolverRejectsTamperedLockIntegrity(t *testing.T) {
	store := testStore(t)
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	if _, err := store.Install(manifest, testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	entry := lock.Plugins["bash"]
	entry.ManifestDigest = testDigest("f")
	lock.Plugins["bash"] = entry
	if err := WriteLock(store.layout.LockPath(), lock); err != nil {
		t.Fatal(err)
	}
	if _, err := NewResolver(store); err == nil {
		t.Fatal("tampered active lock metadata accepted")
	}
}
