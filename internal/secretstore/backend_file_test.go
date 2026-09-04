package secretstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	backend, ok := second.backend.(fileBackend)
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
	}
	if err := second.Set(name, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
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
