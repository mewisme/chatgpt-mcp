package cli

import (
	"bytes"
	"strings"
	"testing"
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

func TestTUICommandRejectsUnknownDeepLinkBeforeLaunch(t *testing.T) {
	command := tuiCommand()
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"missing"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "unknown TUI path") {
		t.Fatalf("unknown TUI path error = %v", err)
	}
}
