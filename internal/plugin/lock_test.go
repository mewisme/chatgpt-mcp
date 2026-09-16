package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteLockAtomicallyReplacesState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.lock.json")
	lock := NewLockFile()
	lock.Plugins["bash"] = LockPlugin{Registry: "official", Publisher: "mewisme", Version: "1.0.0", ManifestDigest: testDigest("a"), ArtifactDigest: testDigest("b"), Enabled: true}
	if err := WriteLock(path, lock); err != nil {
		t.Fatal(err)
	}
	entry := lock.Plugins["bash"]
	entry.Enabled = false
	lock.Plugins["bash"] = entry
	if err := WriteLock(path, lock); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Plugins["bash"].Enabled {
		t.Fatal("atomic replacement did not persist new state")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("unexpected lock directory entries: %#v", entries)
	}
}

func TestLoadLockRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.lock.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"plugins":{"../bash":{}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLock(path); err == nil {
		t.Fatal("corrupt plugin lock accepted")
	}
}

func testDigest(value string) string { return "sha256:" + strings.Repeat(value, 64) }
