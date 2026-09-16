package application

import (
	"context"

	"go.mewis.me/chatgpt-mcp/internal/config"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/redact"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
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

func CFTunnelStatusFromSnapshot(snap cftunnelplugin.Snapshot) *runtimecontrol.CFTunnelStatus {
	mcp := snapshotTarget(cftunnelplugin.TargetMCP, snap.PluginEnabled && snap.DesiredMCP, snap.MCP)
	admin := snapshotTarget(cftunnelplugin.TargetAdmin, snap.PluginEnabled && snap.DesiredAdmin, snap.Admin)
	if !snap.PluginEnabled && !mcp.Desired && !admin.Desired && mcp.LastError == "" && admin.LastError == "" {
		return nil
	}
	return &runtimecontrol.CFTunnelStatus{PluginEnabled: snap.PluginEnabled, Targets: []runtimecontrol.CFTunnelTargetStatus{mcp, admin}}
}

func snapshotTarget(target string, desired bool, endpoint cftunnelplugin.Endpoint) runtimecontrol.CFTunnelTargetStatus {
	item := runtimecontrol.CFTunnelTargetStatus{Target: target, Desired: desired}
	if !desired {
		return item
	}
	if endpoint.AuthErr != nil {
		item.LastError = redact.Text(endpoint.AuthErr.Error())
		return item
	}
	if !endpoint.Ready {
		item.LastError = cftunnelplugin.ErrListenerNotReady.Error()
	}
	return item
}

func MCPExposureError(cfg config.Config) error {
	return tunnelprovider.MCPExposureError(cfg)
}

func AdminExposureError(cfg config.Config) error {
	return tunnelprovider.AdminExposureError(cfg)
}

func StartCFTunnel(ctx context.Context, cfg config.Config, target string) error {
	return StartTunnelProvider(ctx, cfg, "cf", target)
}

func StopCFTunnel(ctx context.Context, target string) error {
	return StopTunnelProvider(ctx, "cf", target)
}
