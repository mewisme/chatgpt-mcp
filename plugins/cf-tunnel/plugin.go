package cftunnel

import pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"

func Plugin() pluginpkg.Builtin {
	return pluginpkg.Builtin{
		ID:             "cf-tunnel",
		Name:           "CF Tunnel",
		Type:           "runtime",
		Description:    "Expose local MCP and Admin HTTP through Cloudflare Quick Tunnels",
		Provides:       []pluginpkg.Capability{"tunnel/cf"},
		Permissions:    []pluginpkg.Permission{pluginpkg.PermissionNetworkOutbound},
		Scopes:         []pluginpkg.PluginScope{pluginpkg.ScopeGlobal},
		Disableable:    true,
		DefaultEnabled: false,
	}
}
