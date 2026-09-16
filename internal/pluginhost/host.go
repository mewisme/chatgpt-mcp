package pluginhost

import (
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func Builtins() pluginpkg.BuiltinRegistry {
	return nil
}

func Attach(store *pluginpkg.Store) {
	if store == nil || store.Layout().EffectiveScope() == pluginpkg.ScopeWorkspace {
		return
	}
	store.Builtins = pluginpkg.CompiledBuiltins()
}
