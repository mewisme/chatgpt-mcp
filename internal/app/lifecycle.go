package app

import (
	"context"
	"errors"
	"sync"
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
	if a.Tools != nil {
		go func() {
			refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := a.Tools.RefreshUpstreams(refreshCtx, false); err != nil && refreshCtx.Err() == nil && a.Logger != nil {
				a.Logger.Warning("UPSTREAM", "upstream.bootstrap.failed", "Initial upstream proxy discovery failed", err)
			}
		}()
	}
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
	if a.MCP != nil {
		if a.Logger != nil {
			a.Logger.Verbose("RUNTIME", "runtime.subscriptions.closing", "Closing MCP subscriptions")
		}
		a.MCP.CloseSubscriptions()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	if a.Tunnel != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.Tunnel.StopContext(ctx); err != nil {
				errCh <- err
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
			if err := a.Upstream.Shutdown(ctx); err != nil {
				if a.Logger != nil {
					a.Logger.Failure("UPSTREAM", "upstream.shutdown.failed", "Upstream shutdown failed", err)
				}
				errCh <- err
			} else if a.Logger != nil {
				a.Logger.Verbose("UPSTREAM", "upstream.stopped", "Upstream servers stopped")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	var stopErr error
	for err := range errCh {
		stopErr = errors.Join(stopErr, err)
	}
	a.runtimeCtx = nil
	a.running = false
	return stopErr
}
