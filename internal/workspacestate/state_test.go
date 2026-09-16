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

func TestLegacyStateMigrationMergesCheckpointsAndKeepsLocalFiles(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	if err := os.MkdirAll(filepath.Join(store.CheckpointRoot(), "data", "cp_local"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.StateRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.StateRoot(), "shell.json"), []byte("local-shell"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.CheckpointRoot(), "index.json"), []byte(`{
  "version": 1,
  "checkpoints": [
    {"id":"cp_local","created_at":"2026-09-16T00:00:00Z","summary":"local"}
  ]
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.CheckpointRoot(), "data", "cp_local", "manifest.json"), []byte("local-manifest"), 0600); err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(filepath.Join(legacy, "checkpoints", "data", "cp_legacy"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "checkpoints", "data", "cp_local"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "shell.json"), []byte("legacy-shell"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "checkpoints", "index.json"), []byte(`{
  "version": 1,
  "checkpoints": [
    {"id":"cp_legacy","created_at":"2026-09-16T01:00:00Z","summary":"legacy"},
    {"id":"cp_local","created_at":"2026-09-16T00:00:00Z","summary":"stale-local"}
  ]
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "checkpoints", "data", "cp_legacy", "manifest.json"), []byte("legacy-manifest"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "checkpoints", "data", "cp_local", "manifest.json"), []byte("stale-manifest"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := store.MigrateLegacyState(legacy); err != nil {
		t.Fatal(err)
	}
	shell, err := os.ReadFile(filepath.Join(store.StateRoot(), "shell.json"))
	if err != nil || string(shell) != "local-shell" {
		t.Fatalf("shell.json=%q err=%v", shell, err)
	}
	index, err := os.ReadFile(filepath.Join(store.CheckpointRoot(), "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `"id": "cp_local"`) || !strings.Contains(string(index), `"id": "cp_legacy"`) || strings.Contains(string(index), "stale-local") {
		t.Fatalf("merged index=%s", index)
	}
	for path, want := range map[string]string{
		filepath.Join(store.CheckpointRoot(), "data", "cp_local", "manifest.json"):  "local-manifest",
		filepath.Join(store.CheckpointRoot(), "data", "cp_legacy", "manifest.json"): "legacy-manifest",
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

func TestGitExcludeWorktreeGitFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repo := t.TempDir()
	if _, err := gitCommand(repo, "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitCommand(repo, "-c", "user.email=test@example.com", "-c", "user.name=test", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "init"); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "worktree")
	if _, err := gitCommand(repo, "worktree", "add", worktree); err != nil {
		t.Fatal(err)
	}
	if err := New(worktree).EnsureGitExcluded(context.Background()); err != nil {
		t.Fatal(err)
	}
	exclude, err := gitCommand(worktree, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil || exclude == "" {
		exclude, err = gitCommand(worktree, "rev-parse", "--git-path", "info/exclude")
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(exclude) {
			exclude = filepath.Join(worktree, exclude)
		}
	}
	data, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/.cgm/") {
		t.Fatalf("worktree exclude missing /.cgm/: %q path=%s", data, exclude)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".cgm", ".gitignore")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created .cgm/.gitignore")
	}
}

func gitCommand(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
