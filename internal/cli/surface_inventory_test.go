package cli

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"go.mewis.me/chatgpt-mcp/internal/capability"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

type cliInventoryCommand struct {
	Path    string   `json:"path"`
	Use     string   `json:"use"`
	Aliases []string `json:"aliases,omitempty"`
	Args    string   `json:"args,omitempty"`
	Flags   []string `json:"flags,omitempty"`
}

func TestSurfaceInventoryCLICommands(t *testing.T) {
	root := newRootCommand()
	commands := collectCLIInventory(root)
	if len(commands) < 40 {
		t.Fatalf("too few public commands: %d", len(commands))
	}
	byPath := map[string]cliInventoryCommand{}
	for _, command := range commands {
		byPath[command.Path] = command
	}
	for _, path := range []string{"doctor", "completion", "tunnel admin update", "plugin install", "tunnel cf start"} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("missing command %q", path)
		}
	}
	install := byPath["plugin install"]
	if !containsFlag(install.Flags, "scope") || !containsFlag(install.Flags, "workspace") || !containsFlag(install.Flags, "portable") {
		t.Fatalf("plugin install flags=%v", install.Flags)
	}
	for _, path := range []string{
		"tunnel attach", "tunnel update", "tunnel managed create", "workspace access add",
		"request approve", "request grant list", "config import", "plugin rollback",
	} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("missing compared command %q", path)
		}
	}
	for _, command := range commands {
		if command.Path == "instruction" || strings.HasPrefix(command.Path, "instruction ") {
			t.Fatalf("CLI instruction namespace should stay TUI/Admin-only: %s", command.Path)
		}
		if strings.HasPrefix(command.Path, "context ") || command.Path == "context" {
			t.Fatalf("CLI project-context namespace should stay TUI/Admin-only: %s", command.Path)
		}
	}
	for _, command := range commands {
		if strings.HasPrefix(command.Path, "plugin ") && (strings.Contains(command.Path, "rule") || strings.Contains(command.Path, "skill")) {
			t.Fatalf("plugin resource lifecycle command should not exist: %s", command.Path)
		}
	}
	testutil.WriteLocalJSON(t, "surface-parity-cli.json", map[string]any{
		"exemptions": publicCapabilityExemptions,
		"commands":   commands,
		"catalog":    capability.All(),
	})
}

func collectCLIInventory(root *cobra.Command) []cliInventoryCommand {
	out := []cliInventoryCommand{}
	var walk func(*cobra.Command, []string, bool)
	walk = func(cmd *cobra.Command, prefix []string, hiddenAncestor bool) {
		hidden := hiddenAncestor || cmd.Hidden
		path := capability.RootPath
		if cmd != root {
			prefix = append(prefix, cmd.Name())
			path = capability.NormalizePath(strings.Join(prefix, " "))
		}
		if cmd.Runnable() && !hidden {
			item := cliInventoryCommand{Path: path, Use: strings.TrimSpace(cmd.Use), Aliases: append([]string(nil), cmd.Aliases...)}
			if cmd.Args != nil {
				item.Args = "custom"
			}
			cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
				if flag.Hidden {
					return
				}
				item.Flags = append(item.Flags, flag.Name)
			})
			sort.Strings(item.Flags)
			out = append(out, item)
		}
		for _, child := range cmd.Commands() {
			walk(child, prefix, hidden)
		}
	}
	walk(root, nil, false)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func containsFlag(flags []string, name string) bool {
	for _, flag := range flags {
		if flag == name {
			return true
		}
	}
	return false
}
