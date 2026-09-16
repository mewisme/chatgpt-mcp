package application

import (
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func CFTunnelSnapshot(cfg config.Config) cftunnelplugin.Snapshot {
	builtin := cftunnelplugin.Plugin()
	layout := pluginpkg.DefaultLayout()
	enabled := builtin.DefaultEnabled
	if loaded, err := layout.LoadConfig(); err == nil {
		if state, ok := loaded.Builtins[builtin.ID]; ok {
			enabled = state.Enabled
		}
	}
	desiredMCP, desiredAdmin := false, false
	settings := pluginpkg.SettingsStore{Layout: layout}
	if values, err := settings.Get(builtin.Schema, builtin.ID); err == nil {
		desiredMCP, _ = values[cftunnelplugin.TargetMCP].(bool)
		desiredAdmin, _ = values[cftunnelplugin.TargetAdmin].(bool)
	}
	return cftunnelplugin.Snapshot{
		PluginEnabled: enabled,
		DesiredMCP:    desiredMCP,
		DesiredAdmin:  desiredAdmin,
		MCP:           cftunnelplugin.Endpoint{Ready: cfg.Server.Enabled && cfg.Server.Port > 0, Port: cfg.Server.Port, AuthErr: MCPExposureError(cfg)},
		Admin:         cftunnelplugin.Endpoint{Ready: cfg.Admin.Enabled && cfg.Admin.Port > 0, Port: cfg.Admin.Port, AuthErr: AdminExposureError(cfg)},
	}
}

func MCPExposureError(cfg config.Config) error {
	if !cfg.Server.Enabled {
		return cftunnelplugin.ErrMCPHTTPDisabled
	}
	if !cfg.Auth.MCPEnabled {
		return cftunnelplugin.ErrMCPAuthDisabled
	}
	if strings.TrimSpace(cfg.Auth.MCPTokenHash) == "" {
		return cftunnelplugin.ErrMCPTokenMissing
	}
	return nil
}

func AdminExposureError(cfg config.Config) error {
	if !cfg.Admin.Enabled {
		return cftunnelplugin.ErrAdminHTTPDisabled
	}
	if !cfg.Auth.AdminEnabled {
		return cftunnelplugin.ErrAdminAuthDisabled
	}
	if strings.TrimSpace(cfg.Auth.AdminTokenHash) == "" {
		return cftunnelplugin.ErrAdminTokenMissing
	}
	return nil
}
