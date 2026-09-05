package checkpoint

import (
	"os"
	"path/filepath"
	"strings"
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

func TestCheckpointRestorePreservesFileAndDirectoryModes(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not expose Unix permission bits consistently")
	}
	root := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0710); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "tool.sh")
	if err := os.WriteFile(file, []byte("before"), 0751); err != nil {
		t.Fatal(err)
	}
	id, err := store.Before("ws_test", root, "delete_directory", []string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore("ws_test", root, id); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0710 || fileInfo.Mode().Perm() != 0751 {
		t.Fatalf("restored modes dir=%#o file=%#o", dirInfo.Mode().Perm(), fileInfo.Mode().Perm())
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

func TestCheckpointMetadataFollowsRootConfigFormat(t *testing.T) {
	stateRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateRoot, "config.yaml"), []byte("server: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	file := filepath.Join(workspaceRoot, "file.txt")
	if err := os.WriteFile(file, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(stateRoot)
	id, err := store.Before("ws_test", workspaceRoot, "write_file", []string{file}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Path("ws_test"), "index.yaml")); err != nil {
		t.Fatalf("checkpoint index did not follow YAML format: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Path("ws_test"), "data", id, "manifest.yaml")); err != nil {
		t.Fatalf("checkpoint manifest did not follow YAML format: %v", err)
	}
}

func TestCheckpointYAMLManifestPreservesColonContent(t *testing.T) {
	stateRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateRoot, "config.yaml"), []byte("server: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	file := filepath.Join(workspaceRoot, "file.txt")
	content := "name: value\nurl: https://example.com\nheader: x:y\nplain"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(stateRoot)
	id, err := store.Before("ws_test", workspaceRoot, "edit_file", []string{file}, false)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(store.Path("ws_test"), "data", id, "manifest.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "content: |") || !strings.Contains(string(manifest), "name: value") {
		t.Fatalf("manifest content is not encoded as a YAML block scalar:\n%s", manifest)
	}
	if _, err := store.PreviewRestore("ws_test", workspaceRoot, id); err != nil {
		t.Fatalf("preview failed to decode YAML checkpoint: %v", err)
	}
	if err := os.WriteFile(file, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore("ws_test", workspaceRoot, id); err != nil {
		t.Fatalf("restore failed to decode YAML checkpoint: %v", err)
	}
	restored, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != content {
		t.Fatalf("restored content = %q, want %q", restored, content)
	}
}
