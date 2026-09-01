package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestSubcommandNamesDoNotUseDashesExceptExplicitNames(t *testing.T) {
	allowed := map[string]bool{"chatgpt-mcp tunnel admin-key": true}
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		for _, child := range command.Commands() {
			if strings.Contains(child.Name(), "-") && !allowed[child.CommandPath()] {
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
		{[]string{"cfg", "preset", "ls"}, "list"},
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
