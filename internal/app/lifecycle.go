package app

import (
	"context"

	"go.mewis.me/chatgpt-mcp/internal/activity"
)

func (a *App) Start(ctx context.Context) error {
	if err := a.Bootstrap(); err != nil {
		return err
	}
	if a.Tunnel == nil {
		return nil
	}
	if err := a.Tunnel.StartContext(ctx); err != nil {
		a.Activity.Publish(activity.Event{Kind: "tunnel.error", Message: err.Error()})
		return err
	}
	if a.Tunnel.Status().Running {
		a.Activity.Publish(activity.Event{Kind: "tunnel.started", Message: a.Tunnel.Status().PublicURL})
	}
	return nil
}

func (a *App) Stop() error {
	if a.Tunnel == nil {
		return nil
	}
	status := a.Tunnel.Status()
	if err := a.Tunnel.Stop(); err != nil {
		a.Activity.Publish(activity.Event{Kind: "tunnel.error", Message: err.Error()})
		return err
	}
	if status.Running {
		a.Activity.Publish(activity.Event{Kind: "tunnel.stopped", Message: status.PublicURL})
	}
	return nil
}
