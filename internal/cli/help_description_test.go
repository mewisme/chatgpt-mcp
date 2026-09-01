package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAllCommandsHaveHelpDescriptions(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if strings.TrimSpace(cmd.Short) == "" {
			t.Errorf("command %q has no Short description", cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(newRootCommand())
}
