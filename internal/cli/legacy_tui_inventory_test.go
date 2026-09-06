package cli

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var legacyInteractiveCommandPaths = []string{
	"mcp server list",
	"request list",
	"tunnel list",
	"workspace list",
}

func TestLegacyInteractiveCommandInventoryIsFrozen(t *testing.T) {
	root := newRootCommand()
	got := make([]string, 0, len(legacyInteractiveCommandPaths))
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		interactiveFlag := command.Flags().Lookup("interactive")
		noInteractiveFlag := command.Flags().Lookup("no-interactive")
		if (interactiveFlag == nil) != (noInteractiveFlag == nil) {
			t.Errorf("legacy interactive flags must remain paired on %q", command.CommandPath())
		}
		if interactiveFlag != nil {
			path := strings.TrimSpace(strings.TrimPrefix(command.CommandPath(), root.Name()))
			got = append(got, path)
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Strings(got)
	want := append([]string(nil), legacyInteractiveCommandPaths...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy interactive command inventory changed: got=%v want=%v; add new interactive UX only under cgm tui", got, want)
	}
}
