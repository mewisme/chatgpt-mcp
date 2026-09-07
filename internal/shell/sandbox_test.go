package shell

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	args, err := bubblewrapArgs(cmd, root, []string{root}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\x00")
	for _, expected := range []string{"--unshare-pid", "--tmpfs\x00/tmp", "--bind\x00" + root + "\x00" + root, "--chdir\x00" + root, "--\x00/bin/sh\x00-c\x00pwd"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("bubblewrap args missing %q: %#v", expected, args)
		}
	}
	if strings.Contains(joined, "--ro-bind\x00/etc\x00/etc") {
		t.Fatalf("sandbox exposed all of /etc: %#v", args)
	}
	if _, err := os.Stat("/etc/passwd"); err == nil && !strings.Contains(joined, "--ro-bind\x00/etc/passwd\x00/etc/passwd") {
		t.Fatalf("sandbox omitted required identity config: %#v", args)
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
	result, err = manager.Exec(ctx, workspaceID, "test ! -e /etc/shadow && test -r /etc/passwd && test -r /etc/hosts && printf ETC_MINIMAL")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stdout != "ETC_MINIMAL" {
		t.Fatalf("sandbox system config surface is not minimal/usable: %#v", result)
	}
}

func TestBubblewrapArgsUnsharesNetworkWhenRequested(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap arguments are Linux-specific")
	}
	root := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", "pwd")
	args, err := bubblewrapArgs(cmd, root, []string{root}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(args, "--unshare-net") {
		t.Fatalf("network namespace missing: %#v", args)
	}
}

func TestShellNetworkIsolationGrantSemantics(t *testing.T) {
	base := context.Background()
	if !shellNetworkIsolated(base, workspace.ShellNetworkAuto, "curl https://example.com") {
		t.Fatal("auto network policy did not isolate unapproved execution")
	}
	for _, code := range []controlguard.Code{controlguard.CodeExternalAccess, controlguard.CodeExternalMutation} {
		ctx := controlguard.WithGrant(base, controlguard.Grant{RequestID: "req_network", Code: code})
		if shellNetworkIsolated(ctx, workspace.ShellNetworkAuto, "curl https://example.com") {
			t.Fatalf("approved %s did not open network", code)
		}
		if !shellNetworkIsolated(ctx, workspace.ShellNetworkDeny, "curl https://example.com") {
			t.Fatalf("deny policy was bypassed by %s", code)
		}
	}
	strict := controlguard.WithGrant(base, controlguard.Grant{RequestID: "req_shell", Code: controlguard.CodeShellExecution})
	if shellNetworkIsolated(strict, workspace.ShellNetworkAuto, "curl https://example.com") {
		t.Fatal("exact approved network command remained isolated")
	}
	if !shellNetworkIsolated(strict, workspace.ShellNetworkAuto, "python script.py") {
		t.Fatal("generic shell approval opened unclassified network")
	}
	host := controlguard.WithGrant(base, controlguard.Grant{RequestID: "req_host", Code: controlguard.CodeHostMutation})
	if shellNetworkIsolated(host, workspace.ShellNetworkAuto, "apt install curl") {
		t.Fatal("approved package-manager command remained isolated")
	}
	if !shellNetworkIsolated(host, workspace.ShellNetworkAuto, "kill 123") || !shellNetworkIsolated(host, workspace.ShellNetworkDeny, "apt install curl") {
		t.Fatal("host approval bypassed network capability or deny policy")
	}
	control := controlguard.WithApproval(base, controlguard.Approval{RequestID: "req_control", Capability: "cap_control", Invocation: controlguard.Invocation{Command: "cgm update"}})
	if shellNetworkIsolated(control, workspace.ShellNetworkAuto, "cgm update") {
		t.Fatal("approved control-plane update remained isolated")
	}
	if !shellNetworkIsolated(control, workspace.ShellNetworkDeny, "cgm update") {
		t.Fatal("control-plane approval bypassed deny policy")
	}
	if shellNetworkIsolated(base, workspace.ShellNetworkInherit, "curl https://example.com") {
		t.Fatal("inherit network policy unexpectedly isolated network")
	}
}

func TestNetworkOnlySandboxBlocksLoopbackUntilExternalApproval(t *testing.T) {
	if runtime.GOOS != "linux" || executableInPath("bwrap", trustedExecutablePath(nil)) == "" {
		t.Skip("bubblewrap unavailable")
	}
	curl := executableInPath("curl", trustedExecutablePath(nil))
	if curl == "" {
		t.Skip("curl unavailable")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("OK")) }))
	defer server.Close()
	cwd := t.TempDir()
	blocked := exec.Command(curl, "--connect-timeout", "1", "--max-time", "1", "-fsS", server.URL)
	blocked, err := wrapShellSandbox(context.Background(), blocked, "curl "+server.URL, cwd, []string{cwd}, nil, workspace.ShellSandboxOff, workspace.ShellNetworkDeny)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := blocked.CombinedOutput(); err == nil {
		t.Fatalf("network deny unexpectedly reached host loopback: %q", output)
	}
	approved := controlguard.WithGrant(context.Background(), controlguard.Grant{RequestID: "req_external", Code: controlguard.CodeExternalAccess})
	allowed := exec.Command(curl, "--connect-timeout", "1", "--max-time", "1", "-fsS", server.URL)
	allowed, err = wrapShellSandbox(approved, allowed, "curl "+server.URL, cwd, []string{cwd}, nil, workspace.ShellSandboxOff, workspace.ShellNetworkAuto)
	if err != nil {
		t.Fatal(err)
	}
	output, err := allowed.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "OK" {
		t.Fatalf("approved network output = %q", output)
	}
}

func TestFilesystemSandboxKeepsApprovedLocalhostNetworkingUsable(t *testing.T) {
	if runtime.GOOS != "linux" || executableInPath("bwrap", trustedExecutablePath(nil)) == "" {
		t.Skip("bubblewrap unavailable")
	}
	curl := executableInPath("curl", trustedExecutablePath(nil))
	if curl == "" {
		t.Skip("curl unavailable")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("OK")) }))
	defer server.Close()
	url := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	cwd := t.TempDir()
	approved := controlguard.WithGrant(context.Background(), controlguard.Grant{RequestID: "req_external", Code: controlguard.CodeExternalAccess})
	cmd := exec.Command(curl, "--connect-timeout", "1", "--max-time", "1", "-fsS", url)
	cmd, err := wrapShellSandbox(approved, cmd, "curl "+url, cwd, []string{cwd}, nil, workspace.ShellSandboxAuto, workspace.ShellNetworkAuto)
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "OK" {
		t.Fatalf("approved sandboxed localhost output = %q", output)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsPath(values []string, target string) bool {
	for _, value := range values {
		if filepath.Clean(value) == filepath.Clean(target) {
			return true
		}
	}
	return false
}
