package caveman

import pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"

func Plugin() pluginpkg.Builtin {
	return pluginpkg.Builtin{
		ID: "caveman", Name: "Caveman", Type: "tool-provider", Description: "Compressed assistant prose while preserving technical meaning",
		Provides: []pluginpkg.Capability{"tool-provider/caveman"}, DefaultEnabled: true,
		Scopes: []pluginpkg.PluginScope{pluginpkg.ScopeGlobal},
		Schema: pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
			{Key: "default_active", Kind: pluginpkg.FieldBool, Title: "Default active", Description: "controls whether Caveman response style is active by default", Default: true},
			{Key: "default_mode", Kind: pluginpkg.FieldEnum, Title: "Default mode", Description: "sets the default Caveman response intensity", Enum: []string{"lite", "full", "ultra", "wenyan-lite", "wenyan-full", "wenyan-ultra"}, Default: "full"},
		}},
	}
}
