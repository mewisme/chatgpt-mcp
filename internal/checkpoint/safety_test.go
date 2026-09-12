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

func TestRestoreRejectsSymlinkSwapAfterValidation(t *testing.T) {
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
	if err := store.ValidateRestorePaths("ws_test", root, id); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.Restore("ws_test", root, id); err == nil {
		t.Fatal("expected restore to reject symlink swap")
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("restore wrote outside workspace: %v", err)
	}
}

func TestSnapshotRejectsSymlinkSwapAfterRootOpen(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require Windows Developer Mode or elevation")
	}
	workspaceRoot := t.TempDir()
	outside := t.TempDir()
	safe := filepath.Join(workspaceRoot, "safe")
	if err := os.Mkdir(safe, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(safe, "file.txt")
	if err := os.WriteFile(path, []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	roots, err := openRestoreRoots([]string{workspaceRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer roots.Close()
	if err := os.RemoveAll(safe); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, safe); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	if _, err := store.snapshot("ws_test", "cp_test", roots, path, 0); err == nil {
		t.Fatal("expected rooted checkpoint snapshot to reject swapped symlink")
	}
}

func TestOpenRestoreRootsRejectsSymlinkRoot(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require Windows Developer Mode or elevation")
	}
	outside := t.TempDir()
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if roots, err := openRestoreRoots([]string{root}); err == nil {
		_ = roots.Close()
		t.Fatal("expected symlink checkpoint root to be rejected")
	}
}
