package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
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
	if !reflect.DeepEqual(got, first) {
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

func TestGlobalAllowDirExtendsWorkspaceScope(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	outside := t.TempDir()
	manager := NewManagerWithGlobalAllowDirs(filepath.Join(t.TempDir(), "workspaces.json"), []string{allowed})
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	expectedAllowed := canonicalRoot(allowed)
	if _, cwd, err := manager.ResolveWorkingDirectory(item.ID, allowed); err != nil || cwd != expectedAllowed {
		t.Fatalf("allowed cwd = %q, want %q err=%v", cwd, expectedAllowed, err)
	}
	path := filepath.Join(allowed, "artifact.txt")
	expectedPath, err := canonicalForContainment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := manager.ResolvePath(item.ID, allowed, path, false); err != nil || got != expectedPath {
		t.Fatalf("allowed path = %q, want %q err=%v", got, expectedPath, err)
	}
	if _, err := manager.ResolvePath(item.ID, root, filepath.Join(outside, "escape.txt"), false); err == nil {
		t.Fatal("unlisted outside path was allowed")
	}
}

func TestWorkspaceAllowDirPersistsAndRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	outside := t.TempDir()
	store := filepath.Join(t.TempDir(), "workspaces.json")
	manager := NewManager(store)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	item, err = manager.AddAllowDir(item.ID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	expectedAllowed := canonicalRoot(allowed)
	if len(item.AllowDirs) != 1 || item.AllowDirs[0] != expectedAllowed {
		t.Fatalf("allow dirs = %#v, want %q", item.AllowDirs, expectedAllowed)
	}
	reloaded := NewManager(store)
	if _, _, err := reloaded.ResolveWorkingDirectory(item.ID, allowed); err != nil {
		t.Fatalf("persisted allow dir rejected: %v", err)
	}
	link := filepath.Join(allowed, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := reloaded.ResolvePath(item.ID, allowed, filepath.Join(link, "file.txt"), false); err == nil {
		t.Fatal("symlink escape from allowed dir was accepted")
	}
	if _, err := reloaded.RemoveAllowDir(item.ID, allowed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reloaded.ResolveWorkingDirectory(item.ID, allowed); err == nil {
		t.Fatal("removed allow dir remained accessible")
	}
}

func TestControlPlaneStateIsExcludedFromWorkspaceScope(t *testing.T) {
	home := t.TempDir()
	controlPlane := filepath.Join(home, ".config", "chatgpt-mcp")
	if err := os.MkdirAll(controlPlane, 0700); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(controlPlane, "workspaces.json"))
	manager.protectedRoot = canonicalRoot(controlPlane)
	item, err := manager.Register(home)
	if err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(home, "project.txt")
	expectedRegular, err := canonicalForContainment(regular, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := manager.ResolvePath(item.ID, home, regular, false); err != nil || got != expectedRegular {
		t.Fatalf("normal workspace path = %q, want %q err=%v", got, expectedRegular, err)
	}
	for _, path := range []string{controlPlane, filepath.Join(controlPlane, "config.json"), filepath.Join(controlPlane, "tunnel.json")} {
		if _, err := manager.ResolvePath(item.ID, home, path, false); err == nil {
			t.Fatalf("protected control-plane path was accessible: %s", path)
		}
	}
	if _, _, err := manager.ResolveWorkingDirectory(item.ID, controlPlane); err == nil {
		t.Fatal("protected control-plane directory was accepted as working directory")
	}
	if _, err := manager.AddAllowDir(item.ID, controlPlane); err == nil {
		t.Fatal("protected control-plane directory was accepted as workspace access grant")
	}
	if _, err := manager.Register(controlPlane); err == nil {
		t.Fatal("protected control-plane directory was accepted as workspace root")
	}
}

func TestDefaultStoreManagerProtectsActiveConfigRoot(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(DefaultStorePath())
	if manager.protectedRoot != canonicalRoot(root) {
		t.Fatalf("protected root = %q, want %q", manager.protectedRoot, canonicalRoot(root))
	}
}
