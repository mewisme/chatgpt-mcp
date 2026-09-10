package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/controlplane"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestShellEnvironmentMarksMCPToolContext(t *testing.T) {
	values := shellEnvironmentMap(context.Background(), workspace.ShellEnvironmentInherit, nil)
	if values[controlplane.ToolContextEnv] != "1" {
		t.Fatalf("tool context = %q", values[controlplane.ToolContextEnv])
	}
	if values[configformat.EnvConfigDir] != configformat.RootPath() {
		t.Fatalf("config root = %q want %q", values[configformat.EnvConfigDir], configformat.RootPath())
	}
}

func TestShellEnvironmentForwardsOnlyContextApproval(t *testing.T) {
	t.Setenv(controlplane.ControlApprovalEnv, "cap_inherited")
	if value := shellEnvironmentMap(context.Background(), workspace.ShellEnvironmentInherit, nil)[controlplane.ControlApprovalEnv]; value != "" {
		t.Fatalf("unapproved shell inherited capability %q", value)
	}
	ctx := controlguard.WithApproval(context.Background(), controlguard.Approval{
		RequestID: "req_test", Capability: "cap_approved", Invocation: controlguard.Invocation{Program: "cgm", Args: []string{"update"}, Command: "cgm update"},
	})
	values := shellEnvironmentMap(ctx, workspace.ShellEnvironmentInherit, nil)
	if values[controlplane.ControlApprovalEnv] != "cap_approved" || values[controlplane.ToolContextEnv] != "1" {
		t.Fatalf("approved shell env = %#v", values)
	}
}

func TestShellEnvironmentPoliciesFilterSecretsAndInjection(t *testing.T) {
	t.Setenv("PATH", "/safe/bin")
	t.Setenv("CUSTOM_VISIBLE", "visible")
	t.Setenv("OPENAI_API_KEY", "secret-key")
	t.Setenv("GITHUB_TOKEN", "secret-token")
	t.Setenv("NODE_OPTIONS", "--require /tmp/inject.js")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")

	inherit := shellEnvironmentMap(context.Background(), workspace.ShellEnvironmentInherit, nil)
	if inherit["OPENAI_API_KEY"] != "secret-key" || inherit["NODE_OPTIONS"] == "" || inherit["CUSTOM_VISIBLE"] != "visible" {
		t.Fatalf("inherit env = %#v", inherit)
	}
	filtered := shellEnvironmentMap(context.Background(), workspace.ShellEnvironmentFiltered, nil)
	for _, name := range []string{"OPENAI_API_KEY", "GITHUB_TOKEN", "NODE_OPTIONS", "SSH_AUTH_SOCK"} {
		if filtered[name] != "" {
			t.Fatalf("filtered environment exposed %s", name)
		}
	}
	pathValue := filtered["PATH"]
	if runtime.GOOS == "windows" {
		pathValue = filtered["Path"]
	}
	if pathValue != "/safe/bin" || filtered["CUSTOM_VISIBLE"] != "visible" {
		t.Fatalf("filtered safe environment = %#v", filtered)
	}
	minimal := shellEnvironmentMap(context.Background(), workspace.ShellEnvironmentMinimal, nil)
	if minimal["PATH"] != strings.Join(trustedExecutablePath(nil), string(os.PathListSeparator)) || minimal["CUSTOM_VISIBLE"] != "" || minimal["OPENAI_API_KEY"] != "" || minimal["NODE_OPTIONS"] != "" {
		t.Fatalf("minimal environment = %#v", minimal)
	}
}

func TestShellEnvironmentExplicitAllowRestoresSelectedVariable(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:secret@example.test/db")
	t.Setenv("NODE_OPTIONS", "--require /tmp/inject.js")
	values := shellEnvironmentMap(context.Background(), workspace.ShellEnvironmentMinimal, []string{"DATABASE_URL"})
	if values["DATABASE_URL"] == "" || values["NODE_OPTIONS"] != "" {
		t.Fatalf("explicit shell environment allow = %#v", values)
	}
}

func TestShellEnvironmentOutputIsDeterministic(t *testing.T) {
	values := shellEnvironment(context.Background(), workspace.ShellEnvironmentMinimal, nil, nil, false)
	for index := 1; index < len(values); index++ {
		if strings.ToUpper(values[index-1]) > strings.ToUpper(values[index]) {
			t.Fatalf("environment is not sorted: %#v", values)
		}
	}
}

func TestStrictShellPathIgnoresUntrustedParentPathShadowing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable shadowing test")
	}
	fakeBin := t.TempDir()
	fakeLS := filepath.Join(fakeBin, "ls")
	if err := os.WriteFile(fakeLS, []byte("#!/bin/sh\necho SHADOWED\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	manager, workspaceID, _ := newShellTestManager(t)
	if err := manager.workspaces.SetShellApprovalPolicy(workspace.ShellApprovalStrict); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Exec(context.Background(), workspaceID, "ls")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Stdout, "SHADOWED") {
		t.Fatalf("strict shell executed PATH-shadowed binary: %#v", result)
	}
}

func TestStrictShellPathAllowsExplicitTrustedShellPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable trusted-path test")
	}
	trustedBin := t.TempDir()
	trustedLS := filepath.Join(trustedBin, "ls")
	if err := os.WriteFile(trustedLS, []byte("#!/bin/sh\necho EXPLICIT_TRUST\n"), 0755); err != nil {
		t.Fatal(err)
	}
	manager, workspaceID, _ := newShellTestManager(t)
	if err := manager.workspaces.SetShellApprovalPolicy(workspace.ShellApprovalStrict); err != nil {
		t.Fatal(err)
	}
	if err := manager.workspaces.SetShellSandboxPolicy(workspace.ShellSandboxOff); err != nil {
		t.Fatal(err)
	}
	if err := manager.workspaces.SetShellNetworkPolicy(workspace.ShellNetworkInherit); err != nil {
		t.Fatal(err)
	}
	manager.workspaces.SetShellPath([]string{trustedBin})
	result, err := manager.Exec(context.Background(), workspaceID, "ls")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Stdout) != "EXPLICIT_TRUST" {
		t.Fatalf("explicit trusted shell path was not used: %#v", result)
	}
}

func TestApprovedControlPlaneCommandUsesCurrentExecutable(t *testing.T) {
	invocation := controlguard.Invocation{Program: "cgm", Args: []string{"config", "set", "server.port", "41001"}, Command: "cgm config set server.port 41001"}
	ctx := controlguard.WithApproval(context.Background(), controlguard.Approval{RequestID: "req_test", Capability: "cap_test", Invocation: invocation})
	cmd, err := commandForPlatform(ctx, invocation.Command)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(cmd.Path) != filepath.Clean(executable) || len(cmd.Args) != len(invocation.Args)+1 {
		t.Fatalf("approved command = path %q args %#v", cmd.Path, cmd.Args)
	}
	for index := range invocation.Args {
		if cmd.Args[index+1] != invocation.Args[index] {
			t.Fatalf("arg %d = %q want %q", index, cmd.Args[index+1], invocation.Args[index])
		}
	}
	if _, err := commandForPlatform(ctx, "cgm config set server.port 41002"); err == nil {
		t.Fatal("changed approved shell command selected current executable")
	}
}

func shellEnvironmentMap(ctx context.Context, policy workspace.ShellEnvironmentPolicy, allow []string) map[string]string {
	values := map[string]string{}
	for _, value := range shellEnvironment(ctx, policy, allow, nil, false) {
		if index := strings.IndexByte(value, '='); index >= 0 {
			values[value[:index]] = value[index+1:]
		}
	}
	return values
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

func TestShellExecReturnsParentCancellationBeforeInternalTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep command test")
	}
	manager, workspaceID, _ := newShellTestManager(t)
	manager.timeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := manager.Exec(ctx, workspaceID, "sleep 1; printf done")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v want parent deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("parent cancellation took %s; child process likely kept shell pipes open", elapsed)
	}
	if strings.Contains(err.Error(), "timed out after 2s") {
		t.Fatalf("parent cancellation was misreported as internal timeout: %v", err)
	}
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
	moved := filepath.Join(child, "moved.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, "cd child"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, "mv file.txt moved.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("destination missing: %v", err)
	}
}

func TestMutationAllowsCWDChangeWithinWorkspace(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(child, "file.txt")
	moved := filepath.Join(child, "moved.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, "cd child && mv file.txt moved.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("destination missing: %v", err)
	}
}

func TestMutationAllowsCWDChangeIntoExplicitAllowedDirectory(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	workspaces := workspace.NewManagerWithGlobalAllowDirs(filepath.Join(t.TempDir(), "workspaces.json"), []string{allowed})
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(workspaces, filepath.Join(t.TempDir(), "state"))
	file := filepath.Join(allowed, "file.txt")
	moved := filepath.Join(allowed, "moved.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), item.ID, "cd "+allowed+" && mv file.txt moved.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("destination missing: %v", err)
	}
}

func TestMutationRejectsCWDChangeOutsideAllowedRoots(t *testing.T) {
	manager, workspaceID, _ := newShellTestManager(t)
	outside := t.TempDir()
	_, err := manager.Exec(context.Background(), workspaceID, "cd "+outside+" && touch file.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestMutationUsesWorkspaceRootByDefault(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	file := filepath.Join(root, "file.txt")
	moved := filepath.Join(root, "moved.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Exec(context.Background(), workspaceID, "mv file.txt moved.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("destination missing: %v", err)
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
