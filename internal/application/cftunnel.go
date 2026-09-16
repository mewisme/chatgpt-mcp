package application

import (
	"context"
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

func StartCFTunnel(ctx context.Context, cfg config.Config, target string) error {
	wantMCP, wantAdmin, err := cfTunnelTargets(target)
	if err != nil {
		return err
	}
	if wantMCP && wantAdmin {
		if err := MCPExposureError(cfg); err != nil {
			return err
		}
		if err := AdminExposureError(cfg); err != nil {
			return err
		}
	} else if wantMCP {
		if err := MCPExposureError(cfg); err != nil {
			return err
		}
	} else if err := AdminExposureError(cfg); err != nil {
		return err
	}
	service, err := NewPluginService()
	if err != nil {
		return err
	}
	if err := service.SetEnabled(ctx, cftunnelplugin.Plugin().ID, true); err != nil {
		return err
	}
	if wantMCP {
		if err := service.SetPluginSetting(ctx, cftunnelplugin.Plugin().ID, cftunnelplugin.TargetMCP, "true"); err != nil {
			return err
		}
	}
	if wantAdmin {
		if err := service.SetPluginSetting(ctx, cftunnelplugin.Plugin().ID, cftunnelplugin.TargetAdmin, "true"); err != nil {
			return err
		}
	}
	return nil
}

func StopCFTunnel(ctx context.Context, target string) error {
	wantMCP, wantAdmin, err := cfTunnelTargets(target)
	if err != nil {
		return err
	}
	service, err := NewPluginService()
	if err != nil {
		return err
	}
	if wantMCP && wantAdmin {
		if err := service.SetEnabled(ctx, cftunnelplugin.Plugin().ID, false); err != nil {
			return err
		}
	}
	if wantMCP {
		if err := service.SetPluginSetting(ctx, cftunnelplugin.Plugin().ID, cftunnelplugin.TargetMCP, "false"); err != nil {
			return err
		}
	}
	if wantAdmin {
		if err := service.SetPluginSetting(ctx, cftunnelplugin.Plugin().ID, cftunnelplugin.TargetAdmin, "false"); err != nil {
			return err
		}
	}
	return nil
}

func cfTunnelTargets(target string) (wantMCP, wantAdmin bool, err error) {
	parsed, err := cftunnelplugin.ParseTarget(target)
	if err != nil {
		return false, false, err
	}
	return parsed == cftunnelplugin.TargetMCP || parsed == "all", parsed == cftunnelplugin.TargetAdmin || parsed == "all", nil
}
