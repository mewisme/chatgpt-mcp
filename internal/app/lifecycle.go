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
	if a.Tools != nil && a.Tools.ClusterNode() != nil {
		a.tunnelLeader = newTunnelLeaderCoordinator(a.Tools.ClusterNode(), a.Tunnel, a.Logger)
		return a.tunnelLeader.Start(ctx)
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
			a.Logger.Verbose("RUNTIME", "runtime.subscriptions.closing", "Closing MCP subscriptions")
		}
		a.MCP.CloseSubscriptions()
	}
	if a.tunnelLeader != nil {
		if err := a.tunnelLeader.Stop(); err != nil {
			first = err
		}
		a.tunnelLeader = nil
	} else if a.Tunnel != nil {
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
	return first
}
