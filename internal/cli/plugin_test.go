package cli

import (
	"io"
	"strings"
	"testing"
)

func TestPluginCommandSurface(t *testing.T) {
	root := newRootCommand()
	for _, args := range [][]string{
		{"plugin", "search"}, {"plugin", "info"}, {"plugin", "list"}, {"plugin", "install"}, {"plugin", "uninstall"},
		{"plugin", "enable"}, {"plugin", "disable"}, {"plugin", "update"}, {"plugin", "outdated"}, {"plugin", "verify"},
		{"plugin", "registry", "list"}, {"plugin", "registry", "add"}, {"plugin", "registry", "remove"},
	} {
		command, _, err := root.Find(args)
		if err != nil {
			t.Fatalf("find %v: %v", args, err)
		}
		if command == nil || command.Name() != args[len(args)-1] {
			t.Fatalf("find %v = %#v", args, command)
		}
	}
}

func TestPluginRegistryAddRequiresPinnedRepository(t *testing.T) {
	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(testCommandArgs(t, "plugin", "registry", "add", "community", "https://plugins.example.test"))
	_, err := root.ExecuteC()
	if err == nil || !strings.Contains(err.Error(), "repository") {
		t.Fatalf("registry add error = %v", err)
	}
}

func TestPluginUninstallExposesForceFlag(t *testing.T) {
	cmd := pluginUninstallCommand()
	if cmd.Flags().Lookup("force") == nil {
		t.Fatal("plugin uninstall is missing --force")
	}
}
