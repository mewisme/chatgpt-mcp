package app

import (
	"context"
	"errors"
	"sync"
	"time"

	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func anyTunnelRunning(statuses []tunnel.Status) bool {
	for _, status := range statuses {
		if status.Enabled && status.Running {
			return true
		}
	}
	return false
}

func (a *App) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracepkg.ObserverFromContext(ctx) == nil && a.trace != nil {
		ctx = tracepkg.WithObserver(ctx, a.trace)
	}
	span := tracepkg.Start(ctx, "APP", "app.runtime.start", "Starting application runtime")
	if err := a.Bootstrap(); err != nil {
		span.FailMessage("Application runtime bootstrap failed", err)
		return err
	}
	if a.Tools != nil && a.Tools.Workspaces != nil {
		workspaceSpan := tracepkg.Start(ctx, "APP", "app.workspaces.activate", "Activating workspace runtime state")
		if err := a.Tools.Workspaces.Activate(); err != nil {
			workspaceSpan.FailMessage("Workspace runtime activation failed", err)
			span.FailMessage("Application runtime start failed", err)
			return err
		}
		workspaces, err := a.Tools.Workspaces.List()
		if err != nil {
			_ = a.Tools.Workspaces.Deactivate()
			workspaceSpan.FailMessage("Workspace runtime validation failed", err)
			span.FailMessage("Application runtime start failed", err)
			return err
		}
		workspaceSpan.EndMessage("Workspace runtime state activated", tracepkg.Int("workspace_count", len(workspaces)))
	}
	a.runtimeCtx = ctx
	if a.Tools != nil {
		go func() {
			refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			refreshSpan := tracepkg.Start(refreshCtx, "APP", "app.upstream.initial-discovery", "Starting initial upstream MCP discovery", tracepkg.Bool("force_refresh", false))
			if err := a.Tools.RefreshUpstreams(refreshCtx, false); err != nil {
				refreshSpan.FailMessage("Initial upstream MCP discovery failed", err)
				if refreshCtx.Err() == nil && a.Logger != nil {
					a.Logger.Warning("UPSTREAM", "upstream.bootstrap.failed", "Initial upstream proxy discovery failed", err)
				}
				return
			}
			refreshSpan.EndMessage("Initial upstream MCP discovery completed")
		}()
	}
	if a.Tunnels != nil {
		tunnelSpan := tracepkg.Start(ctx, "APP", "app.tunnel.start", "Starting tunnel runtime")
		if err := a.Tunnels.StartContext(ctx); err != nil {
			tunnelSpan.FailMessage("Tunnel runtime start failed", err)
			if a.Logger != nil {
				a.Logger.Warning("TUNNEL", "tunnel.start.partial", "One or more tunnels failed to start", err)
			}
			if a.MCP == nil && !anyTunnelRunning(a.Tunnels.Statuses()) {
				span.FailMessage("Application runtime start failed", err)
				a.runtimeCtx = nil
				if a.Tools != nil && a.Tools.Workspaces != nil {
					_ = a.Tools.Workspaces.Deactivate()
				}
				return err
			}
		} else {
			tunnelSpan.EndMessage("Tunnel runtime started", tracepkg.Bool("ready", a.Tunnels.Ready()))
		}
	}
	a.running = true
	span.EndMessage("Application runtime started", tracepkg.Bool("running", true))
	return nil
}

func (a *App) Stop() error {
	span := tracepkg.StartObserver(a.trace, "APP", "app.runtime.stop", "Stopping application runtime")
	if a.Notifications != nil {
		a.Notifications.Stop()
	}
	if a.Bridge != nil {
		_ = a.Bridge.Close()
		a.Bridge = nil
	}
	if a.MCP != nil {
		subscriptionsSpan := tracepkg.StartObserver(a.trace, "APP", "app.mcp.subscriptions.close", "Closing MCP subscriptions")
		if a.Logger != nil {
			a.Logger.Verbose("RUNTIME", "runtime.subscriptions.closing", "Closing MCP subscriptions")
		}
		a.MCP.CloseSubscriptions()
		subscriptionsSpan.EndMessage("MCP subscriptions closed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	if a.Tools != nil && a.Tools.Processes != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.Tools.Processes.Shutdown(ctx); err != nil {
				errCh <- err
			}
		}()
	}
	if a.Tunnels != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tunnelSpan := tracepkg.StartObserver(a.trace, "APP", "app.tunnel.stop", "Stopping tunnel runtime")
			if err := a.Tunnels.StopContext(ctx); err != nil {
				tunnelSpan.FailMessage("Tunnel runtime stop failed", err)
				errCh <- err
			} else {
				tunnelSpan.EndMessage("Tunnel runtime stopped")
			}
		}()
	}
	if a.Upstream != nil {
		if a.Logger != nil {
			a.Logger.Verbose("UPSTREAM", "upstream.stopping", "Stopping upstream servers")
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			upstreamSpan := tracepkg.StartObserver(a.trace, "APP", "app.upstream.shutdown", "Shutting down upstream MCP manager")
			if err := a.Upstream.Shutdown(ctx); err != nil {
				upstreamSpan.FailMessage("Upstream MCP manager shutdown failed", err)
				if a.Logger != nil {
					a.Logger.Failure("UPSTREAM", "upstream.shutdown.failed", "Upstream shutdown failed", err)
				}
				errCh <- err
			} else {
				upstreamSpan.EndMessage("Upstream MCP manager shut down")
				if a.Logger != nil {
					a.Logger.Verbose("UPSTREAM", "upstream.stopped", "Upstream servers stopped")
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	var stopErr error
	for err := range errCh {
		stopErr = errors.Join(stopErr, err)
	}
	if a.Tools != nil && a.Tools.Workspaces != nil {
		workspaceSpan := tracepkg.StartObserver(a.trace, "APP", "app.workspaces.deactivate", "Releasing workspace runtime state")
		if err := a.Tools.Workspaces.Deactivate(); err != nil {
			workspaceSpan.FailMessage("Workspace runtime deactivation failed", err)
			stopErr = errors.Join(stopErr, err)
		} else {
			workspaceSpan.EndMessage("Workspace runtime state released")
		}
	}
	a.runtimeCtx = nil
	a.running = false
	if stopErr != nil {
		span.FailMessage("Application runtime stop failed", stopErr)
	} else {
		span.EndMessage("Application runtime stopped", tracepkg.Bool("running", false))
	}
	return stopErr
}
