package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	if err := WriteFileAtomic(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("content = %q", data)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("unexpected directory entries: %#v", entries)
	}
}

func TestWriteFileAtomicUsesRequestedMode(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not expose Unix permission bits consistently")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteFileAtomic(path, []byte("state"), 0640); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("mode = %#o", info.Mode().Perm())
	}
}

func TestWriteFileAtomicRootCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	path := filepath.Join("nested", "state.json")
	if err := WriteFileAtomicRoot(root, path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomicRoot(root, path, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("content = %q", data)
	}
}

func TestWriteFileAtomicRootRejectsSymlinkEscape(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require Windows Developer Mode or elevation")
	}
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteFileAtomicRoot(root, filepath.Join("escape", "state.json"), []byte("outside"), 0600); err == nil {
		t.Fatal("expected rooted atomic write to reject symlink escape")
	}
	if _, err := os.Stat(filepath.Join(outside, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rooted write escaped state root: %v", err)
	}
}
