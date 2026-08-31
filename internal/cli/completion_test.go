package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestConfigCompletionIncludesKeysAndTypedValues(t *testing.T) {
	keys, directive := completeConfigSet(nil, nil, "per")
	if directive != cobra.ShellCompDirectiveNoFileComp || !hasCompletion(keys, "permissions.allow_dirs") {
		t.Fatalf("key completions = %#v directive=%v", keys, directive)
	}
	values, directive := completeConfigSet(nil, []string{"auth.mcp_enabled"}, "")
	if directive != cobra.ShellCompDirectiveNoFileComp || !hasCompletion(values, "true") || !hasCompletion(values, "false") {
		t.Fatalf("bool completions = %#v directive=%v", values, directive)
	}
	dirs, directive := completeConfigSet(nil, []string{"permissions.allow_dirs"}, "")
	if len(dirs) != 0 || directive != cobra.ShellCompDirectiveFilterDirs {
		t.Fatalf("directory completion = %#v directive=%v", dirs, directive)
	}
	selection, _ := completeConfigSelection(nil, nil, "tunnel")
	if !hasCompletion(selection, "tunnel") || !hasCompletion(selection, "tunnel.enabled") || !hasCompletion(selection, "tunnel.organization_id") {
		t.Fatalf("selection completions = %#v", selection)
	}
}

func TestPresetAndFormatCompletion(t *testing.T) {
	presets, _ := completePresetName(nil, nil, "la")
	if !hasCompletion(presets, "lan") || !hasCompletion(presets, "lan-admin") {
		t.Fatalf("preset completions = %#v", presets)
	}
	formats, _ := completeConfigFormat(nil, nil, "t")
	if len(formats) != 1 || formats[0] != "toml" {
		t.Fatalf("format completions = %#v", formats)
	}
}

func TestDynamicEntityAndSessionCompletionUsesSelectedConfigRoot(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	t.Setenv(configformat.EnvConfigDir, root)
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	registered, err := workspace.NewManager(workspace.DefaultStorePath()).Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	manager := upstream.NewManager(upstream.NewStore(upstream.Path()))
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(upstream.Server{ID: "docs", Name: "Docs MCP", Transport: "http", URL: "https://example.invalid/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	journal, err := runtimeevent.NewJournal(root, runtimeevent.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(runtimeevent.Event{Time: time.Now().UTC(), RunID: "run_completion_test", PID: 42, Level: "info", Name: "server.ready", Message: "Server ready"}); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	workspaces, _ := workspaceCompletions(cmd, "ws_")
	if !hasCompletion(workspaces, registered.ID) {
		t.Fatalf("workspace completions = %#v", workspaces)
	}
	upstreams, _ := completeUpstreamID(cmd, nil, "do")
	if !hasCompletion(upstreams, "docs") {
		t.Fatalf("upstream completions = %#v", upstreams)
	}
	sessions, _ := completeSessionID(cmd, nil, "run_completion")
	if !hasCompletion(sessions, "run_completion_test") {
		t.Fatalf("session completions = %#v", sessions)
	}
}

func TestCGMCompletionScriptUsesAliasName(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CLI_NAME", "cgm")
	cmd := newRootCommand()
	if cmd.Name() != "cgm" {
		t.Fatalf("root name = %q", cmd.Name())
	}
	var output bytes.Buffer
	if err := cmd.GenBashCompletion(&output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "__start_cgm") || strings.Contains(text, "__start_chatgpt-mcp") {
		t.Fatalf("unexpected completion script header")
	}
}

func hasCompletion(values []string, want string) bool {
	for _, value := range values {
		candidate, _, _ := strings.Cut(value, "\t")
		if candidate == want {
			return true
		}
	}
	return false
}
