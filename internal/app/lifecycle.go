package app

import (
	"context"
	"time"
)

func (a *App) Start(ctx context.Context) error {
	if err := a.Bootstrap(); err != nil {
		return err
	}
	if a.Tunnel == nil {
		return nil
	}
	if err := a.Tunnel.StartContext(ctx); err != nil {
		return err
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
		if err := a.Tunnel.Stop(); err != nil {
			first = err
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
