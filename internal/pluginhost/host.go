package pluginhost

import (
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	cavemanplugin "go.mewis.me/chatgpt-mcp/plugins/caveman"
	ponytailplugin "go.mewis.me/chatgpt-mcp/plugins/ponytail"
)

func Builtins() pluginpkg.BuiltinRegistry {
	return pluginpkg.BuiltinRegistry{ponytailplugin.Plugin(), cavemanplugin.Plugin()}
}

func Attach(store *pluginpkg.Store) {
	if store == nil {
		return
	}
	store.Builtins = Builtins()
}
