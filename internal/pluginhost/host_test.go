package pluginhost

import (
	"testing"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestBuiltinsAreEmpty(t *testing.T) {
	registry := Builtins()
	if err := registry.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(registry) != 0 {
		t.Fatalf("compiled builtins = %#v", registry)
	}
}

func TestSyncToolsWithoutPluginsLeavesTurnControllersUnregistered(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	Install()
	runtime := tools.NewRuntime()
	if _, ok := runtime.Registry.Schema("ponytail_turn"); ok {
		t.Fatal("ponytail_turn registered without plugin")
	}
	if _, ok := runtime.Registry.Schema("caveman_turn"); ok {
		t.Fatal("caveman_turn registered without plugin")
	}
}

func TestAttachSkipsWorkspaceStore(t *testing.T) {
	layout, err := pluginpkg.WorkspaceLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	Attach(store)
	if len(store.Builtins) != 0 {
		t.Fatalf("workspace builtins = %#v", store.Builtins)
	}
}
