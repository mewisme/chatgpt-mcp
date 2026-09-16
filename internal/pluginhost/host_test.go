package pluginhost

import (
	"testing"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestBuiltinsIncludePonytailAndCaveman(t *testing.T) {
	registry := Builtins()
	if err := registry.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup("ponytail"); !ok {
		t.Fatal("ponytail builtin missing")
	}
	if _, ok := registry.Lookup("caveman"); !ok {
		t.Fatal("caveman builtin missing")
	}
	cfTunnel, ok := registry.Lookup("cf-tunnel")
	if !ok {
		t.Fatal("cf-tunnel builtin missing")
	}
	if cfTunnel.DefaultEnabled || !cfTunnel.Disableable || cfTunnel.Type != "runtime" {
		t.Fatalf("cf-tunnel builtin = %#v", cfTunnel)
	}
	if len(cfTunnel.Provides) != 1 || cfTunnel.Provides[0] != "tunnel/cf" {
		t.Fatalf("cf-tunnel provides = %#v", cfTunnel.Provides)
	}
	if len(cfTunnel.Permissions) != 1 || cfTunnel.Permissions[0] != pluginpkg.PermissionNetworkOutbound {
		t.Fatalf("cf-tunnel permissions = %#v", cfTunnel.Permissions)
	}
	if !cfTunnel.Lifecycle().Enable || !cfTunnel.Lifecycle().Disable || cfTunnel.Lifecycle().Configure {
		t.Fatalf("cf-tunnel lifecycle = %#v", cfTunnel.Lifecycle())
	}
	ponytail, _ := registry.Lookup("ponytail")
	if ponytail.Lifecycle().Install || ponytail.Lifecycle().Uninstall || !ponytail.Lifecycle().Configure {
		t.Fatalf("ponytail lifecycle = %#v", ponytail.Lifecycle())
	}
	for _, id := range []pluginpkg.PluginID{"ponytail", "caveman", "cf-tunnel"} {
		builtin, _ := registry.Lookup(id)
		scopes := builtin.AllowedScopes()
		if len(scopes) != 1 || scopes[0] != pluginpkg.ScopeGlobal {
			t.Fatalf("%s scopes = %#v", id, scopes)
		}
	}
}

func TestSyncToolsRegistersTurnControllers(t *testing.T) {
	Install()
	runtime := tools.NewRuntime()
	if _, ok := runtime.Registry.Schema("ponytail_turn"); !ok {
		t.Fatal("ponytail_turn missing")
	}
	if _, ok := runtime.Registry.Schema("caveman_turn"); !ok {
		t.Fatal("caveman_turn missing")
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
