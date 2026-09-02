package app

import (
	"context"
	"time"
)

func (a *App) Start(ctx context.Context) error {
	if err := a.Bootstrap(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.runtimeCtx = ctx
	if a.Tunnel != nil {
		if err := a.Tunnel.StartContext(ctx); err != nil {
			a.runtimeCtx = nil
			return err
		}
	}
	a.running = true
	return nil
}

func (a *App) Stop() error {
	var first error
	if a.MCP != nil {
		if a.Logger != nil {
			a.Logger.Verbose("RUNTIME", "runtime.subscriptions.closing", "Closing MCP subscriptions")
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
			a.Logger.Verbose("UPSTREAM", "upstream.stopping", "Stopping upstream servers")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := a.Upstream.Shutdown(ctx)
		cancel()
		if err != nil {
			if a.Logger != nil {
				a.Logger.Failure("UPSTREAM", "upstream.shutdown.failed", "Upstream shutdown failed", err)
			}
			if first == nil {
				first = err
			}
		} else if a.Logger != nil {
			a.Logger.Verbose("UPSTREAM", "upstream.stopped", "Upstream servers stopped")
		}
	}
	a.runtimeCtx = nil
	a.running = false
	return first
}
