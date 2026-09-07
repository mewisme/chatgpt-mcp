package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func serveCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "serve", Short: "Start the MCP server", RunE: runServer}
	addExposeFlag(cmd)
	return cmd
}

func runServer(cmd *cobra.Command, args []string) (runErr error) {
	logCommandStep(cmd, "SERVER", "server.config.loading", "Loading runtime configuration")
	source, err := config.Source()
	if err != nil {
		return err
	}
	if !source.Exists {
		return errors.New("chatgpt-mcp is not initialized; run chatgpt-mcp init")
	}
	cfg, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	if err := applyExposeOverride(cmd, &cfg); err != nil {
		return err
	}
	if err := config.Validate(cfg); err != nil {
		return err
	}
	logCommandDebug(cmd, "SERVER", "server.config.loaded", "Runtime configuration loaded", logger.WithDebug("mcp_http", cfg.Server.Enabled), logger.WithDebug("admin", cfg.Admin.Enabled), logger.WithDebug("tunnel", cfg.Tunnel.Enabled), logger.WithDebug("expose", cfg.Server.Expose.Mode))

	runtimeCtx, runtimeCancel := context.WithCancel(context.WithoutCancel(cmd.Context()))
	defer runtimeCancel()

	logCommandStep(cmd, "NETWORK", "server.listeners.resolving", "Resolving listener plan")
	plan, err := resolveListenerPlan(cfg.Server.Expose)
	if err != nil {
		return err
	}
	logCommandDebug(cmd, "NETWORK", "server.listeners.resolved", "Listener plan resolved", logger.WithDebug("hosts", plan.Hosts), logger.WithDebug("addresses", len(plan.Addresses)))
	startedAt := time.Now().UTC()
	serviceInfo := runtimeServiceInfo(cmd)
	interrupt := newForegroundInterrupt(cmd, false)
	defer interrupt.Close()
	log := commandLogger(cmd)
	defer log.Close()
	metadata := runtimeevent.Metadata{RunID: auth.GenerateToken("run"), PID: os.Getpid(), Managed: serviceInfo.Managed, ServiceID: serviceInfo.ID, ServiceScope: serviceInfo.Scope}
	logCommandStep(cmd, "SESSION", "runtime.journal.opening", "Opening runtime journal")
	journal, err := runtimeevent.NewJournal(config.RootPath(), runtimeevent.Options{Metadata: metadata})
	if err != nil {
		return err
	}
	recorder := runtimeevent.NewRecorder(journal, metadata)
	if err := recorder.Record(runtimeevent.Event{Time: startedAt, Level: "info", Kind: "action", Name: "runtime.session.started", Component: "SESSION", Message: "Runtime session started", Fields: []runtimeevent.Field{{Key: "session", Value: metadata.RunID}, {Key: "mode", Value: runtimeSessionMode(metadata)}, {Key: "config", Value: config.RootPath()}}}); err != nil {
		return err
	}
	defer func() {
		status := "ok"
		if runErr != nil {
			status = "error"
		}
		durationMS := time.Since(startedAt).Milliseconds()
		_ = recorder.Record(runtimeevent.Event{Time: time.Now().UTC(), Level: "info", Kind: "info", Name: "runtime.session.ended", Component: "SESSION", Message: "Runtime session ended", Status: status, DurationMS: durationMS, Fields: []runtimeevent.Field{{Key: "session", Value: metadata.RunID}, {Key: "status", Value: status}, {Key: "duration_ms", Value: durationMS}}})
	}()
	log.AddSink(recorder)
	runtime := app.NewWithLogger(cfg, log)
	var control *runtimeControl
	var bindings *httpBindings
	defer func() {
		runtime.Logger.Verbose("SERVER", "server.runtime.cleanup", "Cleaning up runtime services")
		if err := runtime.Stop(); err != nil {
			runtime.Logger.Failure("SERVER", "server.runtime.cleanup.failed", "Runtime cleanup failed", err)
			if runErr == nil {
				runErr = err
			}
		} else {
			runtime.Logger.Ready("SERVER", "server.stopped", "Server stopped")
		}
		if control != nil {
			if err := control.Close(); err != nil {
				runtime.Logger.Warning("CONTROL", "runtime.control.close-failed", "Runtime control cleanup failed", err)
			}
		}
	}()

	currentCfg, currentPlan := cfg, plan
	var reloadMu sync.Mutex
	runtimeReady := false
	shutdownRequest := make(chan struct{}, 1)
	runtime.Logger.Verbose("NETWORK", "server.listeners.opening", "Opening HTTP listeners")
	bindings, err = openHTTPBindings(cfg, plan)
	if err != nil {
		return err
	}
	defer bindings.CloseUnstarted()
	errCh := make(chan error, max(1, len(bindings.mcpListeners)+len(bindings.adminListeners)))
	reload := func(_ context.Context) (runtimeReloadResult, error) {
		reloadMu.Lock()
		defer reloadMu.Unlock()
		if !runtimeReady {
			return runtimeReloadResult{}, errors.New("runtime is still starting")
		}
		next, err := config.LoadRuntime()
		if err != nil {
			return runtimeReloadResult{}, err
		}
		if err := applyExposeOverride(cmd, &next); err != nil {
			return runtimeReloadResult{}, err
		}
		if err := config.Validate(next); err != nil {
			return runtimeReloadResult{}, err
		}
		nextPlan, err := resolveListenerPlan(next.Server.Expose)
		if err != nil {
			return runtimeReloadResult{}, err
		}
		networkRestarted := !networkConfigEqual(currentCfg, next) || !listenerPlanEqual(currentPlan, nextPlan)
		if !networkRestarted {
			if err := runtime.ReloadConfig(next); err != nil {
				return runtimeReloadResult{}, err
			}
			currentCfg, currentPlan = next, nextPlan
			runtime.Logger.Ready("CONFIG", "config.reloaded", "Configuration reloaded")
			return reloadResult(next, false), nil
		}

		runtime.Logger.Action("SERVER", "server.reloading", "Reloading server listeners")
		if err := bindings.Shutdown(); err != nil {
			runtime.Logger.Warning("NETWORK", "server.reload.shutdown.warning", "Previous listeners did not shut down cleanly", err)
		}
		candidate, err := openHTTPBindings(next, nextPlan)
		if err != nil {
			restored, restoreErr := restoreHTTPBindings(runtime, currentCfg, currentPlan, errCh)
			if restoreErr == nil {
				bindings = restored
			}
			combined := errors.Join(err, restoreErr)
			runtime.Logger.Failure("SERVER", "server.reload.failed", "Server reload failed", combined)
			return runtimeReloadResult{}, combined
		}
		if err := runtime.ReloadConfig(next); err != nil {
			candidate.CloseUnstarted()
			restored, restoreErr := restoreHTTPBindings(runtime, currentCfg, currentPlan, errCh)
			if restoreErr == nil {
				bindings = restored
			}
			combined := errors.Join(err, restoreErr)
			runtime.Logger.Failure("SERVER", "server.reload.failed", "Server reload failed", combined)
			return runtimeReloadResult{}, combined
		}
		candidate.Start(runtime, errCh)
		bindings = candidate
		currentCfg, currentPlan = next, nextPlan
		logReadyEndpoints(runtime.Logger, next, nextPlan)
		return reloadResult(next, true), nil
	}
	status := func() runtimeStatusResult {
		reloadMu.Lock()
		defer reloadMu.Unlock()
		tunnelStatus := runtime.Tunnel.Status()
		return runtimeStatusResult{PID: os.Getpid(), RunID: metadata.RunID, Starting: !runtimeReady, Managed: metadata.Managed, ServiceID: metadata.ServiceID, ServiceScope: metadata.ServiceScope, StartedAt: startedAt, ConfigRoot: config.RootPath(), ServerEnabled: currentCfg.Server.Enabled, ServerPort: currentCfg.Server.Port, AdminEnabled: currentCfg.Admin.Enabled, AdminPort: currentCfg.Admin.Port, Exposure: currentCfg.Server.Expose.Mode, TunnelEnabled: currentCfg.Tunnel.Enabled, TunnelConfigured: tunnel.Configured(currentCfg.Tunnel), TunnelRunning: tunnelStatus.Running, TunnelReady: tunnelStatus.Ready, TunnelRestarting: tunnelStatus.Restarting, TunnelID: strings.TrimSpace(currentCfg.Tunnel.ID), TunnelLastError: tunnelStatus.LastError}
	}
	runtime.Logger.Verbose("CONTROL", "runtime.control.starting", "Starting runtime control endpoint")
	control, err = startRuntimeControl(runtimeControlOptions{RunID: metadata.RunID, Managed: metadata.Managed, ServiceID: metadata.ServiceID, ServiceScope: metadata.ServiceScope, StartedAt: startedAt, Events: recorder.Stream, Reload: reload, ReloadWorkspaces: func() (workspaceReloadResult, error) {
		if err := runtime.Tools.ReloadWorkspaces(); err != nil {
			return workspaceReloadResult{}, err
		}
		items, err := runtime.Tools.Workspaces.List()
		if err != nil {
			return workspaceReloadResult{}, err
		}
		runtime.Logger.Ready("WORKSPACE", "workspace.registry.reloaded", "Workspace registry reloaded", logger.With("count", len(items)))
		return workspaceReloadResult{PID: os.Getpid(), Count: len(items)}, nil
	}, Status: status, Approvals: runtime.Tools.Approvals, Executions: runtime.Tools.Executions, Log: runtime.Logger, Shutdown: func() {
		runtimeCancel()
		select {
		case shutdownRequest <- struct{}{}:
		default:
		}
	}, ClearLogs: journal.Clear})
	if err != nil {
		return err
	}
	runtime.Logger.Diagnostic(logger.Info, "CONTROL", "runtime.control.started", "Runtime control endpoint started", logger.WithDebug("address", control.state.Address), logger.WithDebug("path", control.path))
	runtime.Logger.Verbose("RUNTIME", "runtime.services.starting", "Starting runtime services")
	if err := runtime.Start(runtimeCtx); err != nil {
		return err
	}
	bindings.Start(runtime, errCh)
	runtime.Logger.Verbose("NETWORK", "server.listeners.waiting", "Waiting for HTTP listener readiness")
	if err := waitRuntimeHTTPReady(runtimeCtx, cfg, 3*time.Second); err != nil {
		return errors.Join(err, bindings.Shutdown())
	}
	reloadMu.Lock()
	runtimeReady = true
	reloadMu.Unlock()
	logReadyEndpoints(runtime.Logger, cfg, plan)

	shutdown := func() error {
		reloadMu.Lock()
		defer reloadMu.Unlock()
		runtime.Logger.Action("SERVER", "server.stopping", "Stopping server")
		err := bindings.Shutdown()
		if err != nil {
			runtime.Logger.Failure("SERVER", "server.shutdown.failed", "Server shutdown failed", err)
			return err
		}
		return nil
	}

	select {
	case err := <-errCh:
		if err != nil {
			runtime.Logger.Failure("SERVER", "server.listener.failed", "HTTP listener failed", err)
			return errors.Join(err, shutdown())
		}
		return nil
	case <-interrupt.Context.Done():
		reason := interrupt.Reason()
		if reason == "" {
			reason = "context canceled"
		}
		runtime.Logger.Verbose("SERVER", "server.shutdown.requested", "Shutdown requested", logger.With("reason", reason))
		return shutdown()
	case <-shutdownRequest:
		runtime.Logger.Verbose("SERVER", "server.shutdown.requested", "Shutdown requested", logger.With("reason", "runtime control"))
		return shutdown()
	}
}

func runtimeSessionMode(metadata runtimeevent.Metadata) string {
	if !metadata.Managed {
		return "foreground"
	}
	if metadata.ServiceScope != "" {
		return "managed/" + metadata.ServiceScope
	}
	return "managed"
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
}

func waitRuntimeHTTPReady(parent context.Context, cfg config.Config, timeout time.Duration) error {
	endpoints := []string{}
	if cfg.Server.Enabled {
		endpoints = append(endpoints, endpointURL("127.0.0.1", cfg.Server.Port, "/health"))
	}
	if cfg.Admin.Enabled {
		endpoints = append(endpoints, endpointURL("127.0.0.1", cfg.Admin.Port, "/"))
	}
	if len(endpoints) == 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil}}
	var lastErr error
	for time.Now().Before(deadline) {
		ready := true
		for _, endpoint := range endpoints {
			ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err == nil {
				var response *http.Response
				response, err = client.Do(request)
				if err == nil {
					_ = response.Body.Close()
					if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
						err = fmt.Errorf("HTTP %d", response.StatusCode)
					}
				}
			}
			cancel()
			if err != nil {
				lastErr = fmt.Errorf("%s: %w", endpoint, err)
				ready = false
				break
			}
		}
		if ready {
			return nil
		}
		select {
		case <-parent.Done():
			return parent.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("server listeners did not become ready: %w", lastErr)
	}
	return errors.New("server listeners did not become ready")
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func shutdownServers(servers []*http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var first error
	for _, server := range servers {
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			closeErr := server.Close()
			if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) && first == nil {
				first = errors.Join(err, closeErr)
			}
		}
	}
	return first
}
