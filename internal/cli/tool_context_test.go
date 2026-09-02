package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/controlplane"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestMain(m *testing.M) {
	_, cleanup, err := testutil.IsolateConfigHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	value, exists := os.LookupEnv(controlplane.ToolContextEnv)
	_ = os.Unsetenv(controlplane.ToolContextEnv)
	restoreAncestorContext := controlplane.DisableAncestorContextForTesting()
	code := m.Run()
	restoreAncestorContext()
	if exists {
		_ = os.Setenv(controlplane.ToolContextEnv, value)
	}
	cleanup()
	os.Exit(code)
}

func TestMCPToolContextAllowsOnlyReadOnlyCLICommands(t *testing.T) {
	t.Setenv(controlplane.ToolContextEnv, "1")
	root := newRootCommand()
	for _, path := range [][]string{{"status"}, {"config", "list"}, {"config", "preset", "show"}, {"auth", "status"}, {"workspace", "access", "list"}, {"mcp", "server", "show"}, {"tunnel", "status"}} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := prepareCommand(cmd, nil); err != nil {
			t.Fatalf("read-only command denied: %v: %v", path, err)
		}
	}
	for _, path := range [][]string{{"serve"}, {"up"}, {"down"}, {"_service", "run"}, {"logs", "clear", "--force"}, {"config", "set"}, {"config", "reload"}, {"config", "convert"}, {"auth", "mcp", "create"}, {"workspace", "access", "add"}, {"mcp", "server", "add"}, {"tunnel", "enable"}} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		err = prepareCommand(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "control-plane command denied") {
			t.Fatalf("mutating command was not denied: %v: %v", path, err)
		}
	}
}
