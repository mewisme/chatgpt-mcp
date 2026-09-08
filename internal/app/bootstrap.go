package app

import (
	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/telemetry"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func (a *App) Bootstrap() error {
	if a.Config == nil {
		a.Config = config.NewRuntimeStore(config.Default())
	}
	if a.Tools == nil {
		cfg := a.Config.Snapshot()
		a.Tools = tools.NewRuntimeWithAccess(cfg.Features, cfg.Permissions.AllowDirs, func() (bool, int) {
			current := a.Config.Snapshot()
			return current.Admin.Enabled, current.Admin.Port
		})
	}
	if a.Activity == nil {
		a.Activity = activity.NewStream()
	}
	if a.Logger == nil {
		a.Logger = logger.New(logger.Info)
	}
	telemetry.AttachTools(a.Tools, a.Activity, a.Logger)
	telemetry.AttachApprovals(a.Tools.Approvals, a.Activity, a.Logger)
	a.Upstream = a.Tools.Upstream
	a.syncMCPHTTP(a.Config.Snapshot().Server.Enabled)
	a.attachTunnelLifecycle()
	return nil
}

func (a *App) syncMCPHTTP(enabled bool) {
	if !enabled {
		if a.MCP != nil {
			a.MCP.CloseSubscriptions()
			a.MCP = nil
		}
		return
	}
	if a.MCP == nil {
		a.MCP = mcp.NewHTTPRuntimeWithTools(a.Tools)
	} else if a.MCP.Server == nil {
		a.MCP.Server = mcp.NewRuntimeWithTools(a.Tools)
	} else {
		a.MCP.Server.Tools = a.Tools
	}
	a.MCP.Activity = a.Activity
}
