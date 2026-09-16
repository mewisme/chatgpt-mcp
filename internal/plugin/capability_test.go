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

func TestResolverMergesWorkspacePluginsAndRejectsSameID(t *testing.T) {
	global := testStore(t)
	workspace := testWorkspaceStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if _, err := global.Install(testManifest("alpha", "1.0.0", "shell/alpha"), testPayload(t, "alpha")); err != nil {
		t.Fatal(err)
	}
	if err := global.Activate("alpha", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Install(testScopedManifest("rtk", "1.0.0", "command-wrapper/rtk", ScopeWorkspace), testPayload(t, "rtk")); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Activate("rtk", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	globalOnly, err := NewResolverFromStores(global)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := globalOnly.Resolve("command-wrapper/rtk"); !errors.Is(err, ErrCapabilityNotFound) {
		t.Fatalf("global-only rtk = %v", err)
	}
	merged, err := NewResolverFromStores(global, workspace)
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := merged.Resolve("shell/alpha")
	if err != nil || alpha.Scope != ScopeGlobal {
		t.Fatalf("alpha = %#v %v", alpha, err)
	}
	rtk, err := merged.Resolve("command-wrapper/rtk")
	if err != nil || rtk.Scope != ScopeWorkspace {
		t.Fatalf("rtk = %#v %v", rtk, err)
	}
	if _, err := global.Install(testScopedManifest("rtk", "1.0.0", "command-wrapper/rtk", ScopeGlobal, ScopeWorkspace), testPayload(t, "rtk")); err != nil {
		t.Fatal(err)
	}
	if err := global.Activate("rtk", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	_, err = NewResolverFromStores(global, workspace)
	var conflict ScopeConflictError
	if !errors.As(err, &conflict) || conflict.ID != "rtk" {
		t.Fatalf("same-id conflict = %v", err)
	}
}

func TestResolverAllowsDisabledDuplicateAcrossScopes(t *testing.T) {
	global := testStore(t)
	workspace := testWorkspaceStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	manifest := testScopedManifest("rtk", "1.0.0", "command-wrapper/rtk", ScopeGlobal, ScopeWorkspace)
	if _, err := global.Install(manifest, testPayload(t, "rtk")); err != nil {
		t.Fatal(err)
	}
	if err := global.Activate("rtk", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Install(manifest, testPayload(t, "rtk")); err != nil {
		t.Fatal(err)
	}
	if err := workspace.ActivateWithState("rtk", "1.0.0", trust, false); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewResolverFromStores(global, workspace)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.Resolve("command-wrapper/rtk")
	if err != nil || provider.PluginID != "rtk" {
		t.Fatalf("provider = %#v %v", provider, err)
	}
}

func TestWorkspacePluginCanDependOnGlobalProvider(t *testing.T) {
	global := testStore(t)
	workspace := testWorkspaceStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if _, err := global.Install(testManifest("bash", "1.0.0", "shell/bash"), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := global.Activate("bash", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	consumer := testScopedManifest("consumer", "1.0.0", "formatter/consumer", ScopeWorkspace)
	consumer.Dependencies.Capabilities = []Capability{"shell/bash"}
	if _, err := workspace.Install(consumer, testPayload(t, "consumer")); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Activate("consumer", "1.0.0", trust); err == nil {
		t.Fatal("workspace consumer activated without global peer")
	}
	workspace.SetPeers(global)
	if err := workspace.Activate("consumer", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalPluginCannotDependOnWorkspaceProvider(t *testing.T) {
	global := testStore(t)
	workspace := testWorkspaceStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if _, err := workspace.Install(testScopedManifest("bash", "1.0.0", "shell/bash", ScopeWorkspace), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Activate("bash", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	consumer := testManifest("consumer", "1.0.0", "formatter/consumer")
	consumer.Dependencies.Capabilities = []Capability{"shell/bash"}
	if _, err := global.Install(consumer, testPayload(t, "consumer")); err != nil {
		t.Fatal(err)
	}
	global.SetPeers(workspace)
	if err := global.Activate("consumer", "1.0.0", trust); err == nil {
		t.Fatal("global consumer used workspace provider")
	}
}

func TestActivateRejectsEnabledSameIDAcrossPeers(t *testing.T) {
	global := testStore(t)
	workspace := testWorkspaceStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	manifest := testScopedManifest("rtk", "1.0.0", "command-wrapper/rtk", ScopeGlobal, ScopeWorkspace)
	if _, err := global.Install(manifest, testPayload(t, "rtk")); err != nil {
		t.Fatal(err)
	}
	if err := global.Activate("rtk", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Install(manifest, testPayload(t, "rtk")); err != nil {
		t.Fatal(err)
	}
	workspace.SetPeers(global)
	global.SetPeers(workspace)
	err := workspace.Activate("rtk", "1.0.0", trust)
	var conflict ScopeConflictError
	if !errors.As(err, &conflict) || conflict.ID != "rtk" {
		t.Fatalf("activate conflict = %v", err)
	}
	if err := workspace.ActivateWithState("rtk", "1.0.0", trust, false); err != nil {
		t.Fatal(err)
	}
	if err := workspace.SetEnabled("rtk", true); !errors.As(err, &conflict) {
		t.Fatalf("enable conflict = %v", err)
	}
}
