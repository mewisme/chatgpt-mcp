package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockAtomicPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.lock.json")
	lock := NewLockFile()
	lock.Plugins["bash"] = LockPlugin{Registry: "official", Publisher: "mewisme", Version: "1.0.0", ManifestDigest: "sha256:" + strings.Repeat("a", 64), ArtifactDigest: "sha256:" + strings.Repeat("b", 64), Enabled: true}
	if err := WriteLock(path, lock); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Plugins["bash"].Version != "1.0.0" {
		t.Fatalf("version = %q", loaded.Plugins["bash"].Version)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "plugins.lock.json" {
		t.Fatalf("atomic lock left temporary files: %#v", entries)
	}
}

func TestLoadLockRejectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.lock.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"plugins":{"bash":{"registry":"official"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLock(path); err == nil {
		t.Fatal("corrupt lock accepted")
	}
}
