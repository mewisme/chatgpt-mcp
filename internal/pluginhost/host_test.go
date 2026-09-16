package pluginhost

import "testing"

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
