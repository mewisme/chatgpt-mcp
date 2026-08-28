package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func newShellTestManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	root := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(workspaces, filepath.Join(t.TempDir(), "state")), item.ID, root
}

func TestShellPersistsCWD(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Exec(context.Background(), workspaceID, root, "cd child")
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

func TestMutationRequiresMatchingPersistentCWD(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, root, "cd child"); err != nil {
		t.Fatal(err)
	}
	_, err := manager.Exec(context.Background(), workspaceID, root, "rm file.txt")
	if err == nil || !strings.Contains(err.Error(), "does not match working_directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestMutationRejectsCWDChange(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := manager.Exec(context.Background(), workspaceID, root, "cd child && rm file.txt")
	if err == nil || !strings.Contains(err.Error(), "cwd change") {
		t.Fatalf("error = %v", err)
	}
}

func TestMutationRequiresWorkingDirectory(t *testing.T) {
	manager, workspaceID, _ := newShellTestManager(t)
	_, err := manager.Exec(context.Background(), workspaceID, "", "rm file.txt")
	if err == nil || !strings.Contains(err.Error(), "working_directory is required") {
		t.Fatalf("error = %v", err)
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
