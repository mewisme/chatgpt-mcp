package cftunnel

import pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"

func Plugin() pluginpkg.Builtin {
	return pluginpkg.Builtin{
		ID:          "cf-tunnel",
		Name:        "CF Tunnel",
		Type:        "runtime",
		Description: "Expose local MCP and Admin HTTP through Cloudflare Quick Tunnels",
		Provides:    []pluginpkg.Capability{"tunnel/cf"},
		Permissions: []pluginpkg.Permission{pluginpkg.PermissionNetworkOutbound},
		Schema: pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
			{Key: TargetMCP, Kind: pluginpkg.FieldBool, Title: "Expose MCP HTTP", Description: "Provision a Cloudflare Quick Tunnel to the local MCP HTTP listener", Default: false},
			{Key: TargetAdmin, Kind: pluginpkg.FieldBool, Title: "Expose Admin HTTP", Description: "Provision a Cloudflare Quick Tunnel to the local Admin HTTP listener", Default: false},
		}},
		Scopes:         []pluginpkg.PluginScope{pluginpkg.ScopeGlobal},
		Disableable:    true,
		DefaultEnabled: false,
	}
}
