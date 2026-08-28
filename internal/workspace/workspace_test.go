package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
}

func TestRegisterIsStableAndPersistent(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	first, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Path != second.Path {
		t.Fatalf("unstable registration: %#v %#v", first, second)
	}
	reloaded := NewManager(manager.path)
	got, err := reloaded.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("persisted workspace = %#v, want %#v", got, first)
	}
}

func TestResolveWorkingDirectoryRejectsEscape(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.ResolveWorkingDirectory(item.ID, filepath.Dir(root)); err == nil {
		t.Fatal("expected working directory escape to be rejected")
	}
}

func TestResolvePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolvePath(item.ID, root, filepath.Join("outside-link", "file.txt"), false); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestMutationGuardAllowsWorkspaceLocalRm(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateMutationCommand(item.ID, root, "rm file.txt"); err != nil {
		t.Fatalf("safe rm rejected: %v", err)
	}
}

func TestMutationGuardRejectsCwdChange(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	err = manager.ValidateMutationCommand(item.ID, root, "cd child && rm file.txt")
	if err == nil || !strings.Contains(err.Error(), "cwd change") {
		t.Fatalf("error = %v, want cwd change denial", err)
	}
}

func TestMutationGuardRejectsPopdMutation(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	err = manager.ValidateMutationCommand(item.ID, root, "popd && rm file.txt")
	if err == nil || !strings.Contains(err.Error(), "popd cannot be proven workspace-safe") {
		t.Fatalf("error = %v, want popd fail-closed denial", err)
	}
}

func TestMutationGuardRejectsTargetlessPushdMutation(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	err = manager.ValidateMutationCommand(item.ID, root, "pushd && rm file.txt")
	if err == nil || !strings.Contains(err.Error(), "pushd requires an explicit target") {
		t.Fatalf("error = %v, want targetless pushd denial", err)
	}
}

func TestMutationGuardRejectsOutsidePath(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "file.txt")
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateMutationCommand(item.ID, root, "rm "+outside); err == nil {
		t.Fatal("expected outside rm to be rejected")
	}
}

func TestMutationGuardRejectsNestedShellMutation(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	err = manager.ValidateMutationCommand(item.ID, root, `bash -lc "rm file.txt"`)
	if err == nil || !strings.Contains(err.Error(), "cannot be proven") {
		t.Fatalf("error = %v, want fail-closed denial", err)
	}
}

func TestMutationGuardAllowsWorkspaceLocalMove(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateMutationCommand(item.ID, root, "mv old.txt new.txt"); err != nil {
		t.Fatalf("safe mv rejected: %v", err)
	}
}

func TestMutationGuardRejectsMoveDestinationOutside(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "new.txt")
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateMutationCommand(item.ID, root, "mv old.txt "+outside); err == nil {
		t.Fatal("expected outside mv destination to be rejected")
	}
}
