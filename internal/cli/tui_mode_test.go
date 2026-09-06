package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLegacyInteractiveFlagsAreRemoved(t *testing.T) {
	root := newRootCommand()
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		for _, name := range []string{"interactive", "no-interactive"} {
			if command.Flags().Lookup(name) != nil {
				t.Errorf("legacy --%s flag still registered on %q", name, command.CommandPath())
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
	command, _, err := root.Find([]string{"tui"})
	if err != nil || command == nil || strings.TrimSpace(command.Name()) != "tui" {
		t.Fatalf("explicit tui command unavailable: command=%v err=%v", command, err)
	}
}
