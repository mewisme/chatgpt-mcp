package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestTUICommandIsRegistered(t *testing.T) {
	command, _, err := newRootCommand().Find([]string{"tui"})
	if err != nil || command.Name() != "tui" {
		t.Fatalf("tui command = %v, %v", command, err)
	}
}

func TestTUICommandRequiresTerminal(t *testing.T) {
	command := tuiCommand()
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(nil)
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "requires terminal") {
		t.Fatalf("tui non-terminal error = %v", err)
	}
}

func TestTUICommandMissingPlugin(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	previous := tuiIsTerminal
	tuiIsTerminal = func(io.Reader, io.Writer) bool { return true }
	t.Cleanup(func() { tuiIsTerminal = previous })
	command := tuiCommand()
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"home"})
	err := command.Execute()
	if err == nil || !errors.Is(err, application.ErrTerminalUIMissing) {
		t.Fatalf("missing tui plugin error = %v", err)
	}
}
