package ponytail

import pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"

func Plugin() pluginpkg.Builtin {
	return pluginpkg.Builtin{
		ID: "ponytail", Name: "Ponytail", Type: "tool-provider", Description: "Smallest-correct-solution coding guidance",
		Provides: []pluginpkg.Capability{"tool/ponytail"}, DefaultEnabled: true,
		Schema: pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
			{Key: "default_active", Kind: pluginpkg.FieldBool, Title: "Default active", Description: "controls whether Ponytail guidance is active by default", Default: true},
			{Key: "default_mode", Kind: pluginpkg.FieldEnum, Title: "Default mode", Description: "sets the default Ponytail intensity", Enum: []string{"lite", "full", "ultra"}, Default: "full"},
		}},
	}
}
