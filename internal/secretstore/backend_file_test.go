package secretstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	backend := newFileBackend(t.TempDir()).(*fileBackend)
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

func TestFileBackendRejectsSecretStateSymlinkEscape(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require Windows Developer Mode or elevation")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "state")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := New(root)
	if err := store.Set(Name("oauth", "access-token"), "secret-value"); err == nil {
		t.Fatal("expected secret write through escaped state symlink to fail")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("secret write escaped config root: %#v", entries)
	}
}

func TestFileBackendRejectsSecretFileSymlink(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require Windows Developer Mode or elevation")
	}
	root := t.TempDir()
	store := New(root)
	backend := store.backend.(*fileBackend)
	name := Name("oauth", "access-token")
	path, err := backend.path(store.service, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.secret")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.Get(name); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected symlink secret file to be rejected, got %v", err)
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

func TestConcurrentStoresShareFirstMasterKey(t *testing.T) {
	root := t.TempDir()
	const count = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range count {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			store := New(root)
			<-start
			if err := store.Set(Name("concurrent", fmt.Sprint(index)), fmt.Sprintf("value-%d", index)); err != nil {
				t.Errorf("set %d: %v", index, err)
			}
		}(index)
	}
	close(start)
	wg.Wait()
	fresh := New(root)
	for index := range count {
		want := fmt.Sprintf("value-%d", index)
		got, err := fresh.Get(Name("concurrent", fmt.Sprint(index)))
		if err != nil || got != want {
			t.Fatalf("get %d value=%q want=%q err=%v", index, got, want, err)
		}
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
