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
	var first error
	if a.Tunnel != nil {
		status := a.Tunnel.Status()
		if err := a.Tunnel.Stop(); err != nil {
			if a.Activity != nil {
				a.Activity.Publish(activity.Event{Kind: "tunnel.error", Message: err.Error()})
			}
			first = err
		} else if status.Running && a.Activity != nil {
			a.Activity.Publish(activity.Event{Kind: "tunnel.stopped", Message: status.PublicURL})
		}
	}
	if a.Upstream != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Upstream.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}
