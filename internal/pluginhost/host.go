package pluginhost

import (
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	cavemanplugin "go.mewis.me/chatgpt-mcp/plugins/caveman"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
	ponytailplugin "go.mewis.me/chatgpt-mcp/plugins/ponytail"
)

func Builtins() pluginpkg.BuiltinRegistry {
	return pluginpkg.BuiltinRegistry{ponytailplugin.Plugin(), cavemanplugin.Plugin(), cftunnelplugin.Plugin()}
}

func Attach(store *pluginpkg.Store) {
	if store == nil || store.Layout().EffectiveScope() == pluginpkg.ScopeWorkspace {
		return
	}
	store.Builtins = Builtins()
}
