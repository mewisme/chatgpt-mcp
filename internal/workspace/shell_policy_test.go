package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellPolicyRejectsCommonWriteEscapes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "escape.txt")
	outsideDir := filepath.Join(outside, "new-dir")
	cases := []string{
		"echo escaped > " + outsideFile,
		"echo escaped >> " + outsideFile,
		"cp local.txt " + outsideFile,
		"tee " + outsideFile,
		"truncate -s 0 " + outsideFile,
		"touch " + outsideFile,
		"mkdir " + outsideDir,
		"ln local.txt " + outsideFile,
		"Set-Content -Path " + outsideFile + " -Value escaped",
		"Copy-Item -Path local.txt -Destination " + outsideFile,
	}
	for _, command := range cases {
		t.Run(command, func(t *testing.T) {
			if err := manager.ValidateShellCommand(item.ID, root, command); err == nil {
				t.Fatalf("expected workspace escape to be rejected: %s", command)
			}
		})
	}
}

func TestShellPolicyAllowsWorkspaceLocalWrites(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"echo safe > local.txt",
		"cp source.txt nested/destination.txt",
		"touch local.txt",
		"mkdir nested",
		"Set-Content -Path local.txt -Value /tmp/is-content-not-a-path",
		"Copy-Item -Path source.txt -Destination nested/destination.txt",
	} {
		t.Run(command, func(t *testing.T) {
			if err := manager.ValidateShellCommand(item.ID, root, command); err != nil {
				t.Fatalf("safe command rejected: %v", err)
			}
		})
	}
}

func TestShellPolicyRejectsSymlinkWriteEscape(t *testing.T) {
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
	for _, command := range []string{"echo escaped > outside-link/file.txt", "cp local.txt outside-link/file.txt"} {
		if err := manager.ValidateShellCommand(item.ID, root, command); err == nil {
			t.Fatalf("expected symlink escape denial: %s", command)
		}
	}
}

func TestShellPolicyRejectsDynamicWriteTarget(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{`echo escaped > "$HOME/escape.txt"`, `cp local.txt "$DESTINATION"`} {
		err := manager.ValidateShellCommand(item.ID, root, command)
		if err == nil || !strings.Contains(err.Error(), "dynamic path") {
			t.Fatalf("error = %v, want dynamic path denial", err)
		}
	}
}

func TestShellPolicyRejectsNestedShellMutation(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		`bash -lc "cp a.txt b.txt"`,
		`bash -lc "rm file.txt"`,
		`pwsh -Command "Set-Content -Path file.txt -Value x"`,
	} {
		err := manager.ValidateShellCommand(item.ID, root, command)
		if err == nil || !strings.Contains(err.Error(), "cannot be proven") {
			t.Fatalf("error = %v, want nested mutation fail-closed denial", err)
		}
	}
}

func TestShellPolicyRejectsInlineInterpreterMutation(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		`python -c 'open("escape.txt", "w").write("x")'`,
		`node -e 'require("fs").writeFileSync("escape.txt", "x")'`,
	} {
		err := manager.ValidateShellCommand(item.ID, root, command)
		if err == nil || !strings.Contains(err.Error(), "cannot be proven") {
			t.Fatalf("error = %v, want inline mutation denial", err)
		}
	}
	if err := manager.ValidateShellCommand(item.ID, root, `node -e 'console.log("ok")'`); err != nil {
		t.Fatalf("read-only inline code rejected: %v", err)
	}
}

func TestShellPolicyAllowsNullRedirection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix null device semantics")
	}
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateShellCommand(item.ID, root, "go test ./... > /dev/null"); err != nil {
		t.Fatalf("null redirection rejected: %v", err)
	}
}

func TestMutationDetectionCoversWritesAndNestedShells(t *testing.T) {
	manager := newTestManager(t)
	for _, command := range []string{
		"echo x > file.txt",
		"cp a.txt b.txt",
		"tee file.txt",
		`bash -lc "cp a.txt b.txt"`,
		`python -c 'open("x", "w")'`,
		`Set-Content -Path file.txt -Value x`,
	} {
		if !manager.IsMutationCommand(command) {
			t.Fatalf("expected mutation detection: %s", command)
		}
	}
	for _, command := range []string{"go test ./...", `node -e 'console.log("ok")'`} {
		if manager.IsMutationCommand(command) {
			t.Fatalf("unexpected mutation detection: %s", command)
		}
	}
}
