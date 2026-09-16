package pluginhost

import (
	"testing"

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
	ponytail, _ := registry.Lookup("ponytail")
	if ponytail.Lifecycle().Install || ponytail.Lifecycle().Uninstall || !ponytail.Lifecycle().Configure {
		t.Fatalf("ponytail lifecycle = %#v", ponytail.Lifecycle())
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
