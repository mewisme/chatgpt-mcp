package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestCollapseSandboxRootsRemovesNestedPaths(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	got := collapseSandboxRoots([]string{nested, root, root, other})
	if len(got) != 2 || !containsPath(got, root) || !containsPath(got, other) {
		t.Fatalf("collapsed roots = %#v", got)
	}
}

func TestBubblewrapArgsBindWorkspaceAndPrivateTmp(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap arguments are Linux-specific")
	}
	root := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", "pwd")
	args, err := bubblewrapArgs(cmd, root, []string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\x00")
	for _, expected := range []string{"--unshare-pid", "--tmpfs\x00/tmp", "--bind\x00" + root + "\x00" + root, "--chdir\x00" + root, "--\x00/bin/sh\x00-c\x00pwd"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("bubblewrap args missing %q: %#v", expected, args)
		}
	}
}

func TestSandboxBypassesExplicitHostAndControlPlaneApprovals(t *testing.T) {
	host := controlguard.WithGrant(context.Background(), controlguard.Grant{RequestID: "req_host", Code: controlguard.CodeHostMutation})
	if !sandboxBypassApprovedHostMutation(host) {
		t.Fatal("approved host mutation did not bypass filesystem sandbox")
	}
	control := controlguard.WithApproval(context.Background(), controlguard.Approval{RequestID: "req_control", Capability: "cap_control", Invocation: controlguard.Invocation{Command: "cgm update"}})
	if !sandboxBypassApprovedControlPlane(control) {
		t.Fatal("approved control-plane mutation did not bypass filesystem sandbox")
	}
}

func TestStrictAutoSandboxHidesOutsideWorkspace(t *testing.T) {
	if runtime.GOOS != "linux" || executableInPath("bwrap", trustedExecutablePath(nil)) == "" {
		t.Skip("bubblewrap unavailable")
	}
	manager, workspaceID, root := newShellTestManager(t)
	if err := manager.workspaces.SetShellApprovalPolicy(workspace.ShellApprovalStrict); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("host-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := controlguard.WithGrant(context.Background(), controlguard.Grant{RequestID: "req_shell", Code: controlguard.CodeShellExecution})
	command := "test ! -e '" + outsideFile + "' && printf HIDDEN"
	result, err := manager.Exec(ctx, workspaceID, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stdout != "HIDDEN" {
		t.Fatalf("outside workspace remained visible: %#v", result)
	}
	workspaceFile := filepath.Join(root, "inside.txt")
	result, err = manager.Exec(ctx, workspaceID, "printf VISIBLE > '"+workspaceFile+"' && cat '"+workspaceFile+"'")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stdout != "VISIBLE" {
		t.Fatalf("workspace write/read failed inside sandbox: %#v", result)
	}
}

func containsPath(values []string, target string) bool {
	for _, value := range values {
		if filepath.Clean(value) == filepath.Clean(target) {
			return true
		}
	}
	return false
}
