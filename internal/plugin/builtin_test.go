package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func testBuiltin(id string, capability Capability) Builtin {
	return Builtin{ID: PluginID(id), Name: id, Type: "tool-provider", Provides: []Capability{capability}, DefaultEnabled: true, Description: id + " built-in"}
}

func TestBuiltinRegistryRejectsDuplicatesAndInvalidIDs(t *testing.T) {
	if err := (BuiltinRegistry{testBuiltin("demo", "tool/demo"), testBuiltin("demo", "tool/other")}).Validate(); err == nil {
		t.Fatal("duplicate built-in id accepted")
	}
	if err := (BuiltinRegistry{{ID: "Bad", Name: "Bad", Type: "tool-provider"}}).Validate(); err == nil {
		t.Fatal("invalid built-in id accepted")
	}
}

func TestCatalogMergesBuiltinsAndInstalled(t *testing.T) {
	store := testStore(t)
	store.Builtins = BuiltinRegistry{testBuiltin("ponytail", "tool/ponytail")}
	if _, err := store.Install(testManifest("bash", "1.0.0", "shell/bash"), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	items, err := (Manager{Store: store}).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "bash" || items[1].ID != "ponytail" {
		t.Fatalf("catalog = %#v", items)
	}
	if items[0].Origin != OriginInstalled || !items[0].Lifecycle.Uninstall || items[0].Registry != "official" {
		t.Fatalf("installed entry = %#v", items[0])
	}
	if items[1].Origin != OriginBuiltin || items[1].Lifecycle.Install || items[1].Lifecycle.Uninstall || items[1].Lifecycle.Update || items[1].Lifecycle.Rollback || items[1].Lifecycle.Prune || items[1].Origin.Label() != "Built-in" {
		t.Fatalf("builtin entry = %#v", items[1])
	}
	if len(items[0].Scopes) != 1 || items[0].Scopes[0] != ScopeGlobal || len(items[1].Scopes) != 1 || items[1].Scopes[0] != ScopeGlobal {
		t.Fatalf("catalog scopes = %#v %#v", items[0].Scopes, items[1].Scopes)
	}
}

func TestBuiltinUnknownScopeRejected(t *testing.T) {
	builtin := testBuiltin("demo", "tool/demo")
	builtin.Scopes = []PluginScope{"cluster"}
	if err := (BuiltinRegistry{builtin}).Validate(); err == nil {
		t.Fatal("unknown built-in scope accepted")
	}
}

func TestCatalogPrefersBuiltinOverLockCollision(t *testing.T) {
	store := testStore(t)
	store.Builtins = BuiltinRegistry{testBuiltin("bash", "shell/bash")}
	lock := NewLockFile()
	lock.Plugins["bash"] = LockPlugin{Registry: "official", Publisher: "mewisme", Version: "1.0.0", ManifestDigest: testDigest("a"), ArtifactDigest: testDigest("b"), Enabled: true}
	if err := WriteLock(store.layout.LockPath(), lock); err != nil {
		t.Fatal(err)
	}
	items, err := (Manager{Store: store}).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Origin != OriginBuiltin || items[0].ID != "bash" {
		t.Fatalf("catalog = %#v", items)
	}
}

func TestInstallUninstallUpdateRejectBuiltinIDs(t *testing.T) {
	store := testStore(t)
	store.Builtins = BuiltinRegistry{testBuiltin("ponytail", "tool/ponytail")}
	manager := Manager{Store: store}
	if _, err := manager.Install(context.Background(), "ponytail"); !errors.Is(err, ErrBuiltinPlugin) {
		t.Fatalf("install error = %v", err)
	}
	if _, err := manager.Install(context.Background(), "official/ponytail@1.0.0"); !errors.Is(err, ErrBuiltinPlugin) {
		t.Fatalf("qualified install error = %v", err)
	}
	if err := manager.Uninstall(context.Background(), "ponytail", true); !errors.Is(err, ErrBuiltinPlugin) {
		t.Fatalf("uninstall error = %v", err)
	}
	if _, err := manager.Update(context.Background(), "ponytail"); !errors.Is(err, ErrBuiltinPlugin) {
		t.Fatalf("update error = %v", err)
	}
	if _, err := manager.Rollback(context.Background(), "ponytail", ""); !errors.Is(err, ErrBuiltinPlugin) {
		t.Fatalf("rollback error = %v", err)
	}
	if _, err := manager.PruneVersions("ponytail", 0); !errors.Is(err, ErrBuiltinPlugin) {
		t.Fatalf("prune error = %v", err)
	}
	if err := manager.Verify(context.Background(), "ponytail"); err != nil {
		t.Fatalf("verify builtin: %v", err)
	}
}

func TestResolverIncludesBuiltinProvidersAndConflicts(t *testing.T) {
	store := testStore(t)
	store.Builtins = BuiltinRegistry{testBuiltin("core-bash", "shell/bash")}
	if _, err := store.Install(testManifest("bash", "1.0.0", "shell/bash"), testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve("shell/bash"); err == nil {
		t.Fatal("builtin and installed capability resolved silently")
	} else {
		var conflict CapabilityConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("resolve error = %T %v", err, err)
		}
	}
	store.Builtins[0].DefaultEnabled = false
	resolver, err = NewResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.Resolve("shell/bash")
	if err != nil {
		t.Fatal(err)
	}
	if provider.PluginID != "bash" {
		t.Fatalf("provider = %q", provider.PluginID)
	}
}

func TestDisableableBuiltinEnablePersists(t *testing.T) {
	store := testStore(t)
	store.Builtins = BuiltinRegistry{{ID: "cf-tunnel", Name: "CF Tunnel", Type: "runtime", Disableable: true, DefaultEnabled: false, Description: "test"}}
	if store.BuiltinEnabled("cf-tunnel") {
		t.Fatal("disabled-by-default builtin started enabled")
	}
	if err := store.SetEnabled("cf-tunnel", true); err != nil {
		t.Fatal(err)
	}
	if !store.BuiltinEnabled("cf-tunnel") {
		t.Fatal("enabled builtin not persisted")
	}
	items, err := (Manager{Store: store}).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Enabled {
		t.Fatalf("catalog = %#v", items)
	}
	if err := store.SetEnabled("cf-tunnel", false); err != nil {
		t.Fatal(err)
	}
	if store.BuiltinEnabled("cf-tunnel") {
		t.Fatal("disable did not restore default")
	}
	config, err := store.Layout().LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := config.Builtins["cf-tunnel"]; ok {
		t.Fatalf("default overlay left behind: %#v", config.Builtins)
	}
}

func TestNonDisableableBuiltinEnableRejected(t *testing.T) {
	store := testStore(t)
	store.Builtins = BuiltinRegistry{testBuiltin("ponytail", "tool/ponytail")}
	if err := store.SetEnabled("ponytail", false); err == nil || !errors.Is(err, ErrBuiltinPlugin) {
		t.Fatalf("error = %v", err)
	}
}

func TestDisableableBuiltinRejectedOnWorkspaceStore(t *testing.T) {
	store := testWorkspaceStore(t)
	store.Builtins = BuiltinRegistry{{ID: "cf-tunnel", Name: "CF Tunnel", Type: "runtime", Disableable: true, Description: "test"}}
	if err := store.SetEnabled("cf-tunnel", true); err == nil || !strings.Contains(err.Error(), "global-only") {
		t.Fatalf("error = %v", err)
	}
}
