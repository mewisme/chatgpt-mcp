package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileQuarantinesCorruptLockAndPreservesDesiredState(t *testing.T) {
	store := testStore(t)
	config := NewConfig()
	if err := config.SetDesired("bash", "official", "1.0.0", true); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(store.layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store.layout.LockPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.layout.LockPath(), []byte(`{"schema":1,"plugins":`), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Reconcile(store)
	if err != nil {
		t.Fatal(err)
	}
	if !report.CorruptLock || report.QuarantinePath == "" || !strings.Contains(filepath.Base(report.QuarantinePath), "plugins.lock.json.corrupt-") {
		t.Fatalf("reconcile report = %#v", report)
	}
	if _, err := os.Stat(report.QuarantinePath); err != nil {
		t.Fatalf("quarantined lock missing: %v", err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Plugins) != 0 {
		t.Fatalf("recovered lock = %#v", lock)
	}
	loaded, err := LoadConfig(store.layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if desired, ok := loaded.Desired["bash"]; !ok || !desired.Enabled || desired.Version != "1.0.0" {
		t.Fatalf("desired state changed during recovery: %#v ok=%t", desired, ok)
	}
}

func TestLoadLockMarksInvalidContentAsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.lock.json")
	if err := os.WriteFile(path, []byte(`{"schema":999,"plugins":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLock(path); !errors.Is(err, ErrLockCorrupt) {
		t.Fatalf("invalid lock error = %v", err)
	}
}

func TestReconcileDisablesIntegrityFailureWithoutChangingIntent(t *testing.T) {
	store := testStore(t)
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	if _, err := store.Install(manifest, testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	installed, err := store.Installed("bash", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Name = "tampered"
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Root, "plugin.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Reconcile(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Disabled) != 1 || report.Disabled[0] != "bash" || report.Issues["bash"] == "" {
		t.Fatalf("reconcile report = %#v", report)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["bash"].Enabled {
		t.Fatal("integrity-failed plugin remained enabled")
	}
	config, err := LoadConfig(store.layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if desired := config.Desired["bash"]; !desired.Enabled {
		t.Fatalf("desired intent was disabled: %#v", desired)
	}
}

func TestReconcileDisablesDependentWhenProviderUnavailable(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	provider := testManifest("bash", "1.0.0", "shell/bash")
	if _, err := store.Install(provider, testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	consumer := testManifest("consumer", "1.0.0", "formatter/consumer")
	consumer.Dependencies.Capabilities = []Capability{"shell/bash"}
	if _, err := store.Install(consumer, testPayload(t, "consumer")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("consumer", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	bash := lock.Plugins["bash"]
	bash.ManifestDigest = "sha256:" + strings.Repeat("0", 64)
	lock.Plugins["bash"] = bash
	if err := WriteLock(store.layout.LockPath(), lock); err != nil {
		t.Fatal(err)
	}
	report, err := Reconcile(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Disabled) != 2 || report.Disabled[0] != "bash" || report.Disabled[1] != "consumer" || !strings.Contains(report.Issues["consumer"], "shell/bash") {
		t.Fatalf("reconcile report = %#v", report)
	}
	lock, err = LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["bash"].Enabled || lock.Plugins["consumer"].Enabled {
		t.Fatalf("reconciled lock = %#v", lock)
	}
}
