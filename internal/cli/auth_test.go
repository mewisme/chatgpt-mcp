package cli

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestAuthCommandUsesNestedHierarchy(t *testing.T) {
	cmd := authCommand()
	for _, path := range [][]string{{"mcp", "status"}, {"mcp", "show"}, {"mcp", "copy"}, {"mcp", "rotate"}, {"mcp", "create"}, {"mcp", "enable"}, {"mcp", "disable"}, {"admin", "create"}, {"admin", "enable"}, {"admin", "disable"}} {
		resolved, _, err := cmd.Find(path)
		if err != nil || resolved.Name() != path[len(path)-1] {
			t.Fatalf("auth path %v resolved to %v: %v", path, resolved, err)
		}
	}
	if resolved, _, err := cmd.Find([]string{"mcp-create"}); err == nil && resolved.Name() == "mcp-create" {
		t.Fatal("legacy dashed auth command still exists")
	}
}

func TestAuthCreateAndInitRevealTokensOnce(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	tokenPattern := regexp.MustCompile(`(?m)^\s+Direct MCP HTTP token:\s+(mcp_[A-Za-z0-9_-]{24,})\s*$`)

	var initOut bytes.Buffer
	initCmd := newRootCommand()
	initCmd.SetOut(&initOut)
	initCmd.SetErr(&initOut)
	initCmd.SetArgs([]string{"--config-dir", root, "init"})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	initText := initOut.String()
	if strings.Contains(initText, "<redacted>") {
		t.Fatalf("init redacted one-time tokens: %q", initText)
	}
	if !tokenPattern.MatchString(initText) || !strings.Contains(initText, "admin token:") {
		t.Fatalf("init missing plaintext tokens: %q", initText)
	}

	var authOut bytes.Buffer
	authCmd := newRootCommand()
	authCmd.SetOut(&authOut)
	authCmd.SetErr(&authOut)
	authCmd.SetArgs([]string{"--config-dir", root, "auth", "mcp", "create"})
	if err := authCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	authText := authOut.String()
	if strings.Contains(authText, "<redacted>") {
		t.Fatalf("auth create redacted one-time token: %q", authText)
	}
	if tokenPattern.FindStringSubmatch(authText) == nil {
		t.Fatalf("auth create missing plaintext MCP token: %q", authText)
	}
}

func TestAuthMCPShowCopyStatusAndRotate(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	tokenPattern := regexp.MustCompile(`(?m)^\s+Direct MCP HTTP token:\s+(mcp_[A-Za-z0-9_-]{24,})\s*$`)
	var initOut bytes.Buffer
	initCmd := newRootCommand()
	initCmd.SetOut(&initOut)
	initCmd.SetErr(&initOut)
	initCmd.SetArgs([]string{"--config-dir", root, "init"})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	match := tokenPattern.FindStringSubmatch(initOut.String())
	if match == nil {
		t.Fatalf("init missing token: %q", initOut.String())
	}
	token := match[1]
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		cmd := newRootCommand()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"--config-dir", root}, args...))
		err := cmd.Execute()
		return out.String(), err
	}
	status, err := run("auth", "mcp", "status")
	if err != nil || strings.Contains(status, token) || !strings.Contains(status, "revealable=true") || !strings.Contains(status, "/mcp only") || !strings.Contains(status, "Reuse this token") {
		t.Fatalf("status=%q err=%v", status, err)
	}
	shown, err := run("auth", "mcp", "show")
	if err != nil || !strings.Contains(shown, token) {
		t.Fatalf("show=%q err=%v", shown, err)
	}
	previous := clipboardWriteAll
	t.Cleanup(func() { clipboardWriteAll = previous })
	var copied string
	clipboardWriteAll = func(value string) error { copied = value; return nil }
	copiedOut, err := run("auth", "mcp", "copy")
	if err != nil || copied != token || strings.Contains(copiedOut, token) {
		t.Fatalf("copy out=%q copied=%q err=%v", copiedOut, copied, err)
	}
	rotatedOut, err := run("auth", "mcp", "rotate")
	if err != nil {
		t.Fatal(err)
	}
	rotated := tokenPattern.FindStringSubmatch(rotatedOut)
	if rotated == nil || rotated[1] == token {
		t.Fatalf("rotate did not print a new token: %q", rotatedOut)
	}
}

func TestAuthMCPShowLegacyHashOnly(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "sha256$legacy"
	cfg.Auth.AdminTokenHash = "sha256$admin"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config-dir", root, "auth", "mcp", "show"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "rotate once") {
		t.Fatalf("err=%v out=%q", err, out.String())
	}
}

func TestSubcommandNamesDoNotUseDashes(t *testing.T) {
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		for _, child := range command.Commands() {
			if strings.Contains(child.Name(), "-") {
				t.Errorf("dashed subcommand: %s", child.CommandPath())
			}
			visit(child)
		}
	}
	visit(newRootCommand())
}

func TestUsefulCommandAliasesResolve(t *testing.T) {
	root := newRootCommand()
	for _, test := range []struct {
		path []string
		want string
	}{
		{[]string{"cfg"}, "config"},
		{[]string{"cfg", "ls"}, "list"},
		{[]string{"ws"}, "workspace"},
		{[]string{"ws", "ls"}, "list"},
		{[]string{"ws", "access", "ls"}, "list"},
		{[]string{"mcp", "server", "ls"}, "list"},
		{[]string{"mcp", "server", "st"}, "status"},
		{[]string{"auth", "st"}, "status"},
		{[]string{"auth", "mcp", "st"}, "status"},
		{[]string{"tunnel", "st"}, "status"},
		{[]string{"st"}, "status"},
	} {
		resolved, _, err := root.Find(test.path)
		if err != nil || resolved.Name() != test.want {
			t.Fatalf("alias path %v resolved to %v: %v", test.path, resolved, err)
		}
	}
}
