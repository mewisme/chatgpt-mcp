package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreInstallActivateRollback(t *testing.T) {
	store := testStore(t)
	for _, version := range []Version{"1.0.0", "1.1.0"} {
		manifest := testManifest("bash", string(version), "shell/bash")
		payload := testPayload(t)
		if _, err := store.Install(manifest, payload); err != nil {
			t.Fatal(err)
		}
	}
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
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
	if err := store.RemoveVersion("bash", "1.0.0"); err == nil {
		t.Fatal("active version removed")
	}
	if err := store.RemoveVersion("bash", "1.1.0"); err != nil {
		t.Fatal(err)
	}
}

func TestStoreInstallIsImmutable(t *testing.T) {
	store := testStore(t)
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	payload := testPayload(t)
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(manifest, payload); !errors.Is(err, ErrVersionInstalled) {
		t.Fatalf("second install error = %v", err)
	}
}

func TestStoreRejectsSymlinkPayload(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require Windows elevation")
	}
	store := testStore(t)
	payload := testPayload(t)
	if err := os.Symlink(filepath.Join(payload, "usr", "bin", "bash.exe"), filepath.Join(payload, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.Install(testManifest("bash", "1.0.0", "shell/bash"), payload); err == nil {
		t.Fatal("symlink payload accepted")
	}
}

func TestStoreRequiresTrustedPublisher(t *testing.T) {
	store := testStore(t)
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	if _, err := store.Install(manifest, testPayload(t)); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme"}); err == nil {
		t.Fatal("untrusted plugin activated")
	}
	if err := store.Activate("bash", "1.0.0", ActivationTrust{Registry: "official", Publisher: "other", Trusted: true}); err == nil {
		t.Fatal("publisher mismatch activated")
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	store, err := NewStore(Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}, RuntimeContext{OS: "windows", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testPayload(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "usr", "bin", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func testLockEntry(t *testing.T, store *Store, manifest Manifest, version Version) LockPlugin {
	t.Helper()
	digest, err := ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return LockPlugin{Registry: "official", Publisher: manifest.Publisher, Version: version, ManifestDigest: digest, ArtifactDigest: "sha256:" + strings.Repeat("a", 64), Enabled: true}
}
