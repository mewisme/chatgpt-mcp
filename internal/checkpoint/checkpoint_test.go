package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointBeforeAndRestore(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	id, err := store.Before("ws_test", root, "write_file", []string{file}, false)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("checkpoint id is empty")
	}
	if err := os.WriteFile(file, []byte("after"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := store.Restore("ws_test", root, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Restored) != 1 || result.Restored[0] != file {
		t.Fatalf("unexpected restore result: %#v", result)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("restored content = %q", data)
	}
}

func TestCheckpointNewFileRestoreDeletes(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	file := filepath.Join(root, "new.txt")
	id, err := store.Before("ws_test", root, "write_file", []string{file}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("created"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := store.Restore("ws_test", root, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != file {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestCheckpointDryRunDoesNothing(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	id, err := store.Before("ws_test", root, "edit_file", []string{filepath.Join(root, "x.txt")}, true)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Fatalf("dry-run checkpoint id = %q", id)
	}
	values, err := store.List("ws_test", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("dry-run created checkpoints: %#v", values)
	}
}

func TestCheckpointRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	_, err := store.Before("ws_test", root, "write_file", []string{filepath.Join(t.TempDir(), "x.txt")}, false)
	if err == nil {
		t.Fatal("expected checkpoint path escape to fail")
	}
}
