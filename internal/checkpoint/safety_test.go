package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRestoreRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	dir := filepath.Join(root, "dir")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	id, err := store.Before("ws_test", root, "edit_file", []string{file}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := store.ValidateRestorePaths("ws_test", root, id); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}
