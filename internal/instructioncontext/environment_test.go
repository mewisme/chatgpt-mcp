package instructioncontext

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadEnvironmentSnapshot(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	cwd := filepath.Join(root, "nested")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadEnvironmentSnapshot(EnvironmentOptions{
		WorkspaceID: "ws_test", WorkspaceRoot: root, CWD: cwd, EffectiveRoots: []string{root, allowed, root}, AdminEnabled: true, AdminPort: 37422,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Platform != runtime.GOOS || snapshot.OS != runtime.GOOS || snapshot.Arch != runtime.GOARCH || snapshot.Go != runtime.Version() || snapshot.PID != os.Getpid() {
		t.Fatalf("runtime snapshot = %#v", snapshot)
	}
	if snapshot.WorkspaceID != "ws_test" || snapshot.WorkspaceRoot != root || snapshot.CWD != cwd {
		t.Fatalf("workspace snapshot = %#v", snapshot)
	}
	if len(snapshot.EffectiveRoots) != 2 || snapshot.EffectiveRoots[0] != root || snapshot.EffectiveRoots[1] != allowed {
		t.Fatalf("roots = %#v", snapshot.EffectiveRoots)
	}
	if !snapshot.Admin.Enabled || snapshot.Admin.URL != "http://127.0.0.1:37422/" {
		t.Fatalf("admin = %#v", snapshot.Admin)
	}
}

func TestLoadEnvironmentSnapshotDefaultsCWDAndRoots(t *testing.T) {
	root := t.TempDir()
	snapshot, err := LoadEnvironmentSnapshot(EnvironmentOptions{WorkspaceID: "ws_test", WorkspaceRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CWD != root || len(snapshot.EffectiveRoots) != 1 || snapshot.EffectiveRoots[0] != root {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Admin.Enabled || snapshot.Admin.URL != "" {
		t.Fatalf("admin = %#v", snapshot.Admin)
	}
}

func TestLoadEnvironmentSnapshotRejectsCWDOutsideEffectiveRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	_, err := LoadEnvironmentSnapshot(EnvironmentOptions{WorkspaceID: "ws_test", WorkspaceRoot: root, CWD: outside, EffectiveRoots: []string{root}})
	if err == nil {
		t.Fatal("expected cwd outside effective roots to fail")
	}
}

func TestLoadEnvironmentSnapshotRejectsWorkspaceRootOutsideEffectiveRoots(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	_, err := LoadEnvironmentSnapshot(EnvironmentOptions{WorkspaceID: "ws_test", WorkspaceRoot: root, EffectiveRoots: []string{allowed}})
	if err == nil {
		t.Fatal("expected workspace root outside effective roots to fail")
	}
}

func TestLoadEnvironmentSnapshotValidatesRequiredFieldsAndAdminPort(t *testing.T) {
	root := t.TempDir()
	if _, err := LoadEnvironmentSnapshot(EnvironmentOptions{WorkspaceRoot: root}); err == nil {
		t.Fatal("expected missing workspace id to fail")
	}
	if _, err := LoadEnvironmentSnapshot(EnvironmentOptions{WorkspaceID: "ws_test", WorkspaceRoot: root, AdminEnabled: true, AdminPort: 0}); err == nil {
		t.Fatal("expected invalid admin port to fail")
	}
}

func TestLoadEnvironmentSnapshotCanonicalizesSymlinkPaths(t *testing.T) {
	root := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "workspace")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	snapshot, err := LoadEnvironmentSnapshot(EnvironmentOptions{WorkspaceID: "ws_test", WorkspaceRoot: link, CWD: link, EffectiveRoots: []string{link}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.WorkspaceRoot != root || snapshot.CWD != root || len(snapshot.EffectiveRoots) != 1 || snapshot.EffectiveRoots[0] != root {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
