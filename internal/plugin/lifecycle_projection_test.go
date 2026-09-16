package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleProjectsInstructionResources(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	payload := withInstructionResources(t, testPayload(t, "demo"))
	if _, err := store.Install(testManifest("demo", "1.0.0", "formatter/demo"), payload); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.layout.RulesRoot(), "typescript.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("install projected before activation")
	}
	if err := store.Activate("demo", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	assertProjected(t, store.layout, true)
	if err := store.SetEnabled("demo", false); err != nil {
		t.Fatal(err)
	}
	assertProjected(t, store.layout, false)
	if err := store.SetEnabled("demo", true); err != nil {
		t.Fatal(err)
	}
	assertProjected(t, store.layout, true)
	manager := Manager{Store: store}
	if err := manager.Verify(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(context.Background(), "demo", false); err != nil {
		t.Fatal(err)
	}
	assertProjected(t, store.layout, false)
}

func TestLifecycleEnableRejectsUnmanagedDestination(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if _, err := store.Install(testManifest("demo", "1.0.0", "formatter/demo"), withInstructionResources(t, testPayload(t, "demo"))); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateWithState("demo", "1.0.0", trust, false); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(store.layout.RulesRoot(), "typescript.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("user"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SetEnabled("demo", true); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("enable error = %v", err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["demo"].Enabled {
		t.Fatal("failed enable left plugin enabled")
	}
}

func TestLifecycleUpdateRollbackAndPrune(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	v1 := withInstructionResources(t, testPayload(t, "demo"))
	if _, err := store.Install(testManifest("demo", "1.0.0", "formatter/demo"), v1); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	v2 := testPayload(t, "demo")
	if err := os.MkdirAll(filepath.Join(v2, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v2, "rules", "go.md"), []byte("---\ndescription: Go\n---\nGo.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(testManifest("demo", "2.0.0", "formatter/demo"), v2); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "2.0.0", trust); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.layout.RulesRoot(), "typescript.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("update left old rule")
	}
	if _, err := os.Stat(filepath.Join(store.layout.RulesRoot(), "go.md")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	assertProjected(t, store.layout, true)
	if _, err := os.Stat(filepath.Join(store.layout.RulesRoot(), "go.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rollback left new rule")
	}
	if err := store.Activate("demo", "2.0.0", trust); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store}
	if _, err := manager.PruneVersions("demo", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.layout.RulesRoot(), "go.md")); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleVerifyAndReconcileProjectionHealth(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if _, err := store.Install(testManifest("demo", "1.0.0", "formatter/demo"), withInstructionResources(t, testPayload(t, "demo"))); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store}
	if err := os.Remove(filepath.Join(store.layout.RulesRoot(), "typescript.md")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Verify(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("verify missing = %v", err)
	}
	report, err := Reconcile(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Disabled) != 0 || report.Issues["demo"] == "" {
		t.Fatalf("reconcile = %#v", report)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Plugins["demo"].Enabled {
		t.Fatal("reconcile disabled plugin for projection drift")
	}
}

func TestLifecycleUninstallPreservesDrift(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if _, err := store.Install(testManifest("demo", "1.0.0", "formatter/demo"), withInstructionResources(t, testPayload(t, "demo"))); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(store.layout.RulesRoot(), "typescript.md")
	if err := os.WriteFile(dest, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (Manager{Store: store}).Uninstall(context.Background(), "demo", false); !errors.Is(err, ErrProjectionDrift) {
		t.Fatalf("uninstall error = %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "edited" {
		t.Fatalf("drift dest mutated: %q %v", data, err)
	}
}

func TestLifecycleWorkspaceProjectionStaysLocal(t *testing.T) {
	store := testWorkspaceStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	if _, err := store.Install(testScopedManifest("demo", "1.0.0", "formatter/demo", ScopeWorkspace), withInstructionResources(t, testPayload(t, "demo"))); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.layout.WorkspaceRoot, ".cgm", "rules", "typescript.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(DefaultLayout().RulesRoot(), "typescript.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace projection leaked globally: %v", err)
	}
}

func TestInstallRejectsMalformedInstructionResources(t *testing.T) {
	store := testStore(t)
	payload := testPayload(t, "demo")
	if err := os.MkdirAll(filepath.Join(payload, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "rules", "Bad Name.md"), []byte("no"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(testManifest("demo", "1.0.0", "formatter/demo"), payload); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("malformed install error = %v", err)
	}
}

func withInstructionResources(t *testing.T, payload string) string {
	t.Helper()
	fixture := filepath.Join("testdata", "instruction-resources")
	if err := copyPayloadTree(filepath.Join(fixture, "rules"), filepath.Join(payload, "rules")); err != nil {
		t.Fatal(err)
	}
	if err := copyPayloadTree(filepath.Join(fixture, "skills"), filepath.Join(payload, "skills")); err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertProjected(t *testing.T, layout Layout, present bool) {
	t.Helper()
	rule := filepath.Join(layout.RulesRoot(), "typescript.md")
	skill := filepath.Join(layout.SkillsRoot(), "release-check", "SKILL.md")
	for _, path := range []string{rule, skill} {
		_, err := os.Stat(path)
		if present && err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		if !present && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("still present %s: %v", path, err)
		}
	}
}
