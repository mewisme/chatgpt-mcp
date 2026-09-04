package workspace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/controlguard"
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

func TestShellPolicyAllowsExplicitAllowedDirectoryWrites(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	outside := t.TempDir()
	manager := NewManagerWithGlobalAllowDirs(filepath.Join(t.TempDir(), "workspaces.json"), []string{allowed})
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	allowedFile := filepath.Join(allowed, "artifact.txt")
	for _, command := range []string{"echo ok > " + allowedFile, "touch " + allowedFile, "rm " + allowedFile} {
		if err := manager.ValidateShellCommand(item.ID, root, command); err != nil {
			t.Fatalf("allowed-dir command rejected: %s: %v", command, err)
		}
	}
	if err := manager.ValidateShellCommand(item.ID, root, "touch "+filepath.Join(outside, "escape.txt")); err == nil {
		t.Fatal("write outside effective roots was allowed")
	}
}

func TestShellPolicyBlocksChatGPTMCPControlPlaneMutations(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"cmcp config set permissions.allow_dirs /tmp",
		"cgm config set permissions.allow_dirs /tmp",
		"chatgpt-mcp auth mcp create",
		"cmcp workspace access add ws_test /tmp",
		"cgm update",
	} {
		err := manager.ValidateShellCommand(item.ID, root, command)
		guard, ok := controlguard.As(err)
		if err == nil || !ok || guard.Code != controlguard.CodeControlPlaneMutation || !guard.Approvable || guard.Invocation == nil || guard.Invocation.Command != command {
			t.Fatalf("control-plane mutation was not denied: %s: %v", command, err)
		}
	}
	for _, command := range []string{"exec cmcp auth admin disable", `bash -lc "cmcp config preset apply lan"`, `cgm update && echo done`} {
		err := manager.ValidateShellCommand(item.ID, root, command)
		guard, ok := controlguard.As(err)
		if err == nil || !ok || guard.Code != controlguard.CodeControlPlaneMutation || guard.Approvable || guard.Invocation != nil {
			t.Fatalf("wrapped control-plane mutation became approvable: %s: %#v / %v", command, guard, err)
		}
	}
	for _, command := range []string{
		"cmcp status",
		"cgm status",
		"cmcp config list",
		"chatgpt-mcp auth status",
		"cmcp workspace access list ws_test",
	} {
		if err := manager.ValidateShellCommand(item.ID, root, command); err != nil {
			t.Fatalf("read-only control-plane command rejected: %s: %v", command, err)
		}
	}
}

func TestShellPolicyAllowsOnlyExactApprovedDirectControlPlaneInvocation(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	command := "cgm config set server.port 41001"
	invocation, ok := DirectControlPlaneInvocation(command)
	if !ok || invocation == nil {
		t.Fatalf("invocation = %#v ok=%t", invocation, ok)
	}
	ctx := controlguard.WithApproval(context.Background(), controlguard.Approval{RequestID: "req_test", Capability: "cap_test", Invocation: *invocation})
	if err := manager.ValidateShellCommandContext(ctx, item.ID, root, command); err != nil {
		t.Fatalf("exact approved invocation denied: %v", err)
	}
	for _, changed := range []string{"cgm config set server.port 41002", "cgm config set server.port 41001 && echo done", `bash -lc "cgm config set server.port 41001"`} {
		err := manager.ValidateShellCommandContext(ctx, item.ID, root, changed)
		guard, typed := controlguard.As(err)
		if err == nil || !typed || guard.Code != controlguard.CodeControlPlaneMutation {
			t.Fatalf("changed invocation bypassed guard: %q -> %#v / %v", changed, guard, err)
		}
	}
}

func TestShellPolicyNeverApprovesRequestOrServiceCommands(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"cgm request approve req_test", "cgm request deny req_test", "cgm req accept req_test", "cgm req allow req_test", "cgm req reject req_test", "cgm _service run"} {
		err := manager.ValidateShellCommand(item.ID, root, command)
		guard, ok := controlguard.As(err)
		if err == nil || !ok || guard.Code != controlguard.CodeControlPlaneMutation || guard.Approvable || guard.Invocation != nil {
			t.Fatalf("hard-denied command became approvable: %q -> %#v / %v", command, guard, err)
		}
	}
	for _, command := range []string{"cgm request list", "cgm request view req_test", "cgm req ls", "cgm req info req_test"} {
		if err := manager.ValidateShellCommand(item.ID, root, command); err != nil {
			t.Fatalf("read-only request command rejected: %q -> %v", command, err)
		}
	}
}

func TestShellPolicyBlocksToolContextClearing(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t)
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"unset CHATGPT_MCP_TOOL_CONTEXT",
		"env -u CHATGPT_MCP_TOOL_CONTEXT go test ./...",
		"env --unset=CHATGPT_MCP_TOOL_CONTEXT node test.js",
		`python -c 'import os; os.environ.pop("CHATGPT_MCP_TOOL_CONTEXT", None)'`,
		`node -e 'delete process.env.CHATGPT_MCP_TOOL_CONTEXT'`,
		"Remove-Item Env:CHATGPT_MCP_TOOL_CONTEXT",
	} {
		err := manager.ValidateShellCommand(item.ID, root, command)
		guard, ok := controlguard.As(err)
		if err == nil || !ok || guard.Code != controlguard.CodeContextTamper || guard.Approvable || !strings.Contains(err.Error(), "cannot be cleared") {
			t.Fatalf("tool context clearing was not denied: %s: %v", command, err)
		}
	}
}

func TestShellPolicyBlocksProtectedControlPlaneReads(t *testing.T) {
	home := t.TempDir()
	controlPlane := filepath.Join(home, ".config", "chatgpt-mcp")
	if err := os.MkdirAll(controlPlane, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	manager := NewManager(filepath.Join(controlPlane, "workspaces.json"))
	manager.protectedRoot = canonicalRoot(controlPlane)
	item, err := manager.Register(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"cat " + filepath.Join(controlPlane, ".runtime-control.json"),
		"cat .config/chatgpt-mcp/config.json",
		`cat "$HOME/.config/chatgpt-mcp/config.json"`,
		"Get-Content -Path " + filepath.Join(controlPlane, "config.json"),
		`bash -lc "cat .config/chatgpt-mcp/config.json"`,
		`python -c 'print(open("` + filepath.ToSlash(filepath.Join(controlPlane, "config.json")) + `").read())'`,
	} {
		err := manager.ValidateShellCommand(item.ID, home, command)
		guard, ok := controlguard.As(err)
		if err == nil || !ok || guard.Code != controlguard.CodeProtectedState || guard.Approvable || !strings.Contains(err.Error(), "control-plane state access denied") {
			t.Fatalf("protected read was not denied: %s: %v", command, err)
		}
	}
	if err := manager.ValidateShellCommand(item.ID, home, "cat README.md"); err != nil {
		t.Fatalf("normal workspace read rejected: %v", err)
	}
}

func TestShellPolicyBlocksProtectedReadThroughPathAlias(t *testing.T) {
	base := t.TempDir()
	realHome := filepath.Join(base, "real")
	aliasHome := filepath.Join(base, "alias")
	controlPlane := filepath.Join(realHome, ".config", "chatgpt-mcp")
	if err := os.MkdirAll(controlPlane, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realHome, aliasHome); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	manager := NewManager(filepath.Join(controlPlane, "workspaces.json"))
	manager.protectedRoot = canonicalRoot(controlPlane)
	item, err := manager.Register(realHome)
	if err != nil {
		t.Fatal(err)
	}
	aliasedConfig := filepath.ToSlash(filepath.Join(aliasHome, ".config", "chatgpt-mcp", "config.json"))
	command := `python -c 'print(open("` + aliasedConfig + `").read())'`
	err = manager.ValidateShellCommand(item.ID, realHome, command)
	if err == nil || !strings.Contains(err.Error(), "control-plane state access denied") {
		t.Fatalf("aliased protected read was not denied: %v", err)
	}
}
