package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/controlplane"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestShellEnvironmentMarksMCPToolContext(t *testing.T) {
	values := shellEnvironmentMap(context.Background())
	if values[controlplane.ToolContextEnv] != "1" {
		t.Fatalf("tool context = %q", values[controlplane.ToolContextEnv])
	}
	if values[configformat.EnvConfigDir] != configformat.RootPath() {
		t.Fatalf("config root = %q want %q", values[configformat.EnvConfigDir], configformat.RootPath())
	}
}

func TestShellEnvironmentForwardsOnlyContextApproval(t *testing.T) {
	t.Setenv(controlplane.ControlApprovalEnv, "cap_inherited")
	if value := shellEnvironmentMap(context.Background())[controlplane.ControlApprovalEnv]; value != "" {
		t.Fatalf("unapproved shell inherited capability %q", value)
	}
	ctx := controlguard.WithApproval(context.Background(), controlguard.Approval{
		RequestID: "req_test", Capability: "cap_approved", Invocation: controlguard.Invocation{Program: "cgm", Args: []string{"update"}, Command: "cgm update"},
	})
	values := shellEnvironmentMap(ctx)
	if values[controlplane.ControlApprovalEnv] != "cap_approved" || values[controlplane.ToolContextEnv] != "1" {
		t.Fatalf("approved shell env = %#v", values)
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

func shellEnvironmentMap(ctx context.Context) map[string]string {
	values := map[string]string{}
	for _, value := range shellEnvironment(ctx) {
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
