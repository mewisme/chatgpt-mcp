package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/controlplane"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestShellEnvironmentMarksMCPToolContext(t *testing.T) {
	want := controlplane.ToolContextEnv + "=1"
	for _, value := range shellEnvironment() {
		if value == want {
			return
		}
	}
	t.Fatalf("shell environment missing %s", want)
}

func newShellTestManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	root := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(workspaces, filepath.Join(t.TempDir(), "state")), item.ID, item.Path
}

func TestShellPersistsCWD(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Exec(context.Background(), workspaceID, "cd child")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(result.CWD) != filepath.Clean(child) {
		t.Fatalf("cwd = %q, want %q", result.CWD, child)
	}
	status, err := manager.Status(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(status.CWD) != filepath.Clean(child) {
		t.Fatalf("persisted cwd = %q, want %q", status.CWD, child)
	}

	reloaded := NewManager(manager.workspaces, manager.root)
	status, err = reloaded.Status(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(status.CWD) != filepath.Clean(child) {
		t.Fatalf("disk cwd = %q, want %q", status.CWD, child)
	}
}

func TestMutationUsesPersistentCWD(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(child, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, "cd child"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, "rm file.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestMutationRejectsCWDChange(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := manager.Exec(context.Background(), workspaceID, "cd child && rm file.txt")
	if err == nil || !strings.Contains(err.Error(), "cwd change") {
		t.Fatalf("error = %v", err)
	}
}

func TestMutationUsesWorkspaceRootByDefault(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, "rm file.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestShellReset(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reset(workspaceID, child); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(status.CWD) != filepath.Clean(child) {
		t.Fatalf("cwd = %q", status.CWD)
	}
}

func TestShellStateFollowsRootConfigFormat(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("[server]\nport = 37421\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(root, "workspaces.toml"))
	item, err := workspaces.Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(workspaces, root)
	if _, err := manager.Status(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces", item.ID, "shell.toml")); err != nil {
		t.Fatalf("shell state did not follow TOML format: %v", err)
	}
}
