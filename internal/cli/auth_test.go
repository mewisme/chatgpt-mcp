package cli

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestAuthCommandUsesNestedHierarchy(t *testing.T) {
	cmd := authCommand()
	for _, path := range [][]string{{"mcp", "create"}, {"mcp", "enable"}, {"mcp", "disable"}, {"admin", "create"}, {"admin", "enable"}, {"admin", "disable"}} {
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
	tokenPattern := regexp.MustCompile(`(?m)^\s+(mcp token|MCP):\s+(mcp_[A-Za-z0-9_-]{24,})\s*$`)

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
		{[]string{"tunnel", "st"}, "status"},
		{[]string{"st"}, "status"},
	} {
		resolved, _, err := root.Find(test.path)
		if err != nil || resolved.Name() != test.want {
			t.Fatalf("alias path %v resolved to %v: %v", test.path, resolved, err)
		}
	}
}
