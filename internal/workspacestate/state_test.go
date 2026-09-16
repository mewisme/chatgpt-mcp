package workspacestate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityPersistsAndRejectsMismatch(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	first, created, err := store.EnsureIdentity("")
	if err != nil || !created {
		t.Fatalf("created=%t err=%v", created, err)
	}
	second, created, err := store.EnsureIdentity("")
	if err != nil || created || first.ID != second.ID {
		t.Fatalf("second=%#v created=%t err=%v", second, created, err)
	}
	if _, _, err := store.EnsureIdentity("ws_other"); err == nil {
		t.Fatal("identity mismatch was accepted")
	}
}

func TestCorruptIdentityIsNotRegenerated(t *testing.T) {
	store := New(t.TempDir())
	if err := os.MkdirAll(store.Root(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.IdentityPath(), []byte(`{"version":1,"id":"broken"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnsureIdentity(""); err == nil {
		t.Fatal("corrupt identity was regenerated")
	}
}

func TestLegacyStateMigrationIsResumeSafe(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(filepath.Join(legacy, "checkpoints", "data"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]string{
		filepath.Join(legacy, "MEMORY.md"):                        "memory",
		filepath.Join(legacy, "shell.json"):                       "shell",
		filepath.Join(legacy, "checkpoints", "data", "value.txt"): "checkpoint",
		filepath.Join(legacy, "other.json"):                       "other",
	} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := New(root)
	if err := store.MigrateLegacyState(legacy); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		filepath.Join(store.MemoryRoot(), "MEMORY.md"):             "memory",
		filepath.Join(store.StateRoot(), "shell.json"):             "shell",
		filepath.Join(store.CheckpointRoot(), "data", "value.txt"): "checkpoint",
		filepath.Join(store.StateRoot(), "other.json"):             "other",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("path=%s data=%q err=%v", path, data, err)
		}
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state remains: %v", err)
	}
}

func TestGitExcludeRootAndNestedWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repo := t.TempDir()
	if result, err := gitCommand(repo, "init"); err != nil || result != "" {
		_ = result
		if err != nil {
			t.Fatal(err)
		}
	}
	nested := filepath.Join(repo, "tools", "demo")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := New(repo).EnsureGitExcluded(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := New(nested).EnsureGitExcluded(context.Background()); err != nil {
		t.Fatal(err)
	}
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for _, pattern := range []string{"/.cgm/", "/tools/demo/.cgm/"} {
		count := 0
		for _, line := range lines {
			if strings.TrimSpace(line) == pattern {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("pattern %q count=%d in %q", pattern, count, data)
		}
	}
	if err := New(nested).EnsureGitExcluded(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "/tools/demo/.cgm/" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("nested exclude duplicated: %q", data)
	}
}

func gitCommand(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
