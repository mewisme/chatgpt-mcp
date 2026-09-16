package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestPluginCommandSurface(t *testing.T) {
	root := newRootCommand()
	for _, args := range [][]string{
		{"plugin", "search"}, {"plugin", "info"}, {"plugin", "list"}, {"plugin", "install"}, {"plugin", "uninstall"},
		{"plugin", "enable"}, {"plugin", "disable"}, {"plugin", "update"}, {"plugin", "outdated"}, {"plugin", "verify"},
		{"plugin", "config", "list"}, {"plugin", "config", "get"}, {"plugin", "config", "set"}, {"plugin", "config", "reset"},
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

func TestPluginConfigSetGetReset(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	run := func(args ...string) (string, error) {
		t.Helper()
		root := newRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append([]string{"--config-dir", configDir}, args...))
		_, err := root.ExecuteC()
		return out.String(), err
	}
	if _, err := run("plugin", "config", "set", "ponytail", "default_active", "false"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("plugin", "config", "set", "ponytail", "default_mode", "ultra"); err != nil {
		t.Fatal(err)
	}
	out, err := run("plugin", "config", "get", "ponytail", "default_active")
	if err != nil || !strings.Contains(out, "default_active") || !strings.Contains(out, "false") {
		t.Fatalf("get active = %q %v", out, err)
	}
	out, err = run("plugin", "config", "list", "ponytail")
	if err != nil || !strings.Contains(out, "default_mode") || !strings.Contains(out, "ultra") {
		t.Fatalf("list = %q %v", out, err)
	}
	if _, err := run("plugin", "config", "set", "ponytail", "default_mode", "review"); err == nil || !strings.Contains(err.Error(), "valid option") {
		t.Fatalf("session-only mode error = %v", err)
	}
	if _, err := run("plugin", "config", "get", "missing", "default_active"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("unknown plugin error = %v", err)
	}
	if _, err := run("plugin", "config", "reset", "ponytail"); err != nil {
		t.Fatal(err)
	}
	out, err = run("plugin", "config", "get", "ponytail", "default_active")
	if err != nil || !strings.Contains(out, "true") {
		t.Fatalf("reset get = %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "plugins", "config", "ponytail.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reset left plugin config file")
	}
}
