package app

import (
	"context"
	"errors"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func (a *App) Start(ctx context.Context) error {
	if err := a.Bootstrap(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.runtimeCtx = ctx
	cfg := a.Config.Snapshot()
	if err := a.startCluster(ctx, cfg.Cluster); err != nil {
		a.runtimeCtx = nil
		return err
	}
	if a.Tunnel != nil {
		if a.Tools != nil && a.Tools.ClusterNode() != nil {
			a.tunnelLeader = newTunnelLeaderCoordinator(a.Tools.ClusterNode(), a.Tunnel, a.Logger)
			if err := a.tunnelLeader.Start(ctx); err != nil {
				a.tunnelLeader = nil
				_ = a.stopCluster()
				a.runtimeCtx = nil
				return err
			}
		} else if err := a.Tunnel.StartContext(ctx); err != nil {
			_ = a.stopCluster()
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
	if err := a.stopCluster(); err != nil && first == nil {
		first = err
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

type clusterRuntimeConfig struct {
	cluster              config.ClusterConfig
	tunnel               tunnel.Config
	tunnelRuntimeChanged bool
}

func (a *App) restartClusterRuntime(previous, next clusterRuntimeConfig) error {
	ctx := a.runtimeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.stopTunnelRuntime(); err != nil {
		return err
	}
	if err := a.stopCluster(); err != nil {
		return err
	}
	if a.Tunnel != nil && previous.tunnelRuntimeChanged {
		if err := a.Tunnel.Configure(next.tunnel); err != nil {
			return errors.Join(err, a.restoreClusterRuntime(ctx, previous))
		}
	}
	if err := a.startCluster(ctx, next.cluster); err != nil {
		return errors.Join(err, a.restoreClusterRuntime(ctx, previous))
	}
	if err := a.startTunnelRuntime(ctx); err != nil {
		return errors.Join(err, a.restoreClusterRuntime(ctx, previous))
	}
	return nil
}

func (a *App) restoreClusterRuntime(ctx context.Context, previous clusterRuntimeConfig) error {
	var result error
	result = errors.Join(result, a.stopTunnelRuntime(), a.stopCluster())
	if a.Tunnel != nil && previous.tunnelRuntimeChanged {
		result = errors.Join(result, a.Tunnel.Configure(previous.tunnel))
	}
	if err := a.startCluster(ctx, previous.cluster); err != nil {
		return errors.Join(result, err)
	}
	return errors.Join(result, a.startTunnelRuntime(ctx))
}

func (a *App) startTunnelRuntime(ctx context.Context) error {
	if a.Tunnel == nil {
		return nil
	}
	if a.Tools != nil && a.Tools.ClusterNode() != nil {
		a.tunnelLeader = newTunnelLeaderCoordinator(a.Tools.ClusterNode(), a.Tunnel, a.Logger)
		if err := a.tunnelLeader.Start(ctx); err != nil {
			a.tunnelLeader = nil
			return err
		}
		return nil
	}
	return a.Tunnel.StartContext(ctx)
}

func (a *App) stopTunnelRuntime() error {
	if a.tunnelLeader != nil {
		err := a.tunnelLeader.Stop()
		a.tunnelLeader = nil
		return err
	}
	if a.Tunnel != nil {
		return a.Tunnel.Stop()
	}
	return nil
}
