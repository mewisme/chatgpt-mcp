package cli

import (
	"bytes"
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

func TestRootWithoutArgsShowsHelp(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Usage:") || !strings.Contains(text, "serve") || !strings.Contains(text, "status") {
		t.Fatalf("help=%q", text)
	}
}
