package application

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestPluginServiceCatalogIncludesBuiltins(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	service, err := NewPluginService()
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.Installed()
	if err != nil {
		t.Fatal(err)
	}
	found := map[pluginpkg.PluginID]InstalledPluginInfo{}
	for _, item := range items {
		found[item.ID] = item
	}
	for _, id := range []pluginpkg.PluginID{"ponytail", "caveman"} {
		item, ok := found[id]
		if !ok || item.Origin != pluginpkg.OriginBuiltin || item.Lifecycle.Install || item.Lifecycle.Uninstall || item.Lifecycle.Update {
			t.Fatalf("%s = %#v ok=%t", id, item, ok)
		}
	}
}
