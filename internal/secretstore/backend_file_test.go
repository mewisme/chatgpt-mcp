package secretstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileBackendRoundTripAndPersistence(t *testing.T) {
	root := t.TempDir()
	name := Name("tunnel", "runtime-key")
	first := New(root)
	if err := first.Set(name, "secret-value"); err != nil {
		t.Fatal(err)
	}
	second := New(root)
	value, err := second.Get(name)
	if err != nil || value != "secret-value" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	backend, ok := second.backend.(*fileBackend)
	if !ok {
		t.Fatalf("backend=%T", second.backend)
	}
	path, err := backend.path(second.service, name)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(root, "state", "secrets") {
		t.Fatalf("secret path=%q", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !isEncryptedBlob(raw) {
		t.Fatalf("secret file was not encrypted: %q", raw)
	}
	if strings.Contains(string(raw), "secret-value") {
		t.Fatalf("plaintext leaked into encrypted secret file: %q", raw)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("secret mode=%#o want 0600", info.Mode().Perm())
		}
		dirInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if dirInfo.Mode().Perm() != 0700 {
			t.Fatalf("secret directory mode=%#o want 0700", dirInfo.Mode().Perm())
		}
		keyInfo, err := os.Stat(filepath.Join(root, "state", "secrets", masterKeyName))
		if err != nil {
			t.Fatal(err)
		}
		if keyInfo.Mode().Perm() != 0600 {
			t.Fatalf("master key mode=%#o want 0600", keyInfo.Mode().Perm())
		}
	}
	if err := second.Set(name, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestFileBackendReadsLegacyPlaintextAndRewritesEncrypted(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	backend := store.backend.(*fileBackend)
	name := Name("tunnel", "admin-key")
	path, err := backend.path(store.service, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("legacy-plaintext"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := store.Get(name)
	if err != nil || value != "legacy-plaintext" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !isEncryptedBlob(raw) {
		t.Fatalf("legacy secret was not rewritten encrypted: %q", raw)
	}
	if strings.Contains(string(raw), "legacy-plaintext") {
		t.Fatalf("plaintext remained on disk: %q", raw)
	}
	again, err := New(root).Get(name)
	if err != nil || again != "legacy-plaintext" {
		t.Fatalf("reloaded value=%q err=%v", again, err)
	}
}

func TestMigratePlaintextEncryptsLegacyFiles(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	backend := store.backend.(*fileBackend)
	path := filepath.Join(backend.root, "aabbccdd.secret")
	if err := os.MkdirAll(backend.root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("migrate-me"), 0600); err != nil {
		t.Fatal(err)
	}
	migrated, err := store.MigratePlaintext()
	if err != nil || migrated != 1 {
		t.Fatalf("migrated=%d err=%v", migrated, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !isEncryptedBlob(raw) || strings.Contains(string(raw), "migrate-me") {
		t.Fatalf("migrate left plaintext: %q", raw)
	}
	second, err := store.MigratePlaintext()
	if err != nil || second != 0 {
		t.Fatalf("second migrate=%d err=%v", second, err)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	backend := &fileBackend{root: t.TempDir()}
	sealed, err := backend.seal([]byte("round-trip-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if !isEncryptedBlob(sealed) {
		t.Fatalf("sealed=%q", sealed)
	}
	opened, err := backend.open(sealed)
	if err != nil || string(opened) != "round-trip-secret" {
		t.Fatalf("opened=%q err=%v", opened, err)
	}
}

func TestFileBackendIsolatesConfigRoots(t *testing.T) {
	name := Name("oauth", "alpha", "access-token")
	left, right := New(t.TempDir()), New(t.TempDir())
	if err := left.Set(name, "left"); err != nil {
		t.Fatal(err)
	}
	if _, err := right.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("right err=%v", err)
	}
}

func TestSecretMarkerCompatibility(t *testing.T) {
	if !IsMarker(Marker) || !IsMarker(LegacyMarker) {
		t.Fatalf("markers are not recognized")
	}
	if Marker == LegacyMarker || Marker != "<secret-file>" || LegacyMarker != "<os-keyring>" {
		t.Fatalf("marker=%q legacy=%q", Marker, LegacyMarker)
	}
}
