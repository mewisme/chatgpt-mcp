package app

import (
	"context"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/activity"
)

func (a *App) Start(ctx context.Context) error {
	if err := a.Bootstrap(); err != nil {
		return err
	}
	if a.Tunnel == nil {
		return nil
	}
	tunnelConfig := a.Tunnel.Config()
	if tunnelConfig.Enabled && a.Logger != nil {
		a.Logger.Info("TUNNEL", "starting OpenAI Secure MCP Tunnel", "tunnel_id", tunnelConfig.ID)
	}
	if err := a.Tunnel.StartContext(ctx); err != nil {
		if a.Logger != nil {
			a.Logger.Error("TUNNEL", "failed to start", "error", err)
		}
		a.Activity.Publish(activity.Event{Kind: "tunnel.error", Message: err.Error()})
		return err
	}
	status := a.Tunnel.Status()
	if status.Running {
		a.Activity.Publish(activity.Event{Kind: "tunnel.started", Message: status.ID})
	}
	return nil
}

func (a *App) Stop() error {
	var first error
	if a.MCP != nil {
		if a.Logger != nil {
			a.Logger.Info("RUNTIME", "closing MCP subscriptions")
		}
		a.MCP.CloseSubscriptions()
	}
	if a.Tunnel != nil {
		status := a.Tunnel.Status()
		if status.Running && a.Logger != nil {
			a.Logger.Info("TUNNEL", "stopping", "tunnel_id", status.ID)
		}
		if err := a.Tunnel.Stop(); err != nil {
			if a.Logger != nil {
				a.Logger.Error("TUNNEL", "failed to stop", "error", err)
			}
			if a.Activity != nil {
				a.Activity.Publish(activity.Event{Kind: "tunnel.error", Message: err.Error()})
			}
			first = err
		} else if status.Running {
			if a.Logger != nil {
				a.Logger.Success("TUNNEL", "stopped", "tunnel_id", status.ID)
			}
			if a.Activity != nil {
				a.Activity.Publish(activity.Event{Kind: "tunnel.stopped", Message: status.ID})
			}
		}
	}
	if a.Upstream != nil {
		if a.Logger != nil {
			a.Logger.Info("UPSTREAM", "stopping upstream servers")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := a.Upstream.Shutdown(ctx)
		cancel()
		if err != nil {
			if a.Logger != nil {
				a.Logger.Error("UPSTREAM", "shutdown failed", "error", err)
			}
			if first == nil {
				first = err
			}
		} else if a.Logger != nil {
			a.Logger.Success("UPSTREAM", "stopped")
		}
	}
	return first
}
