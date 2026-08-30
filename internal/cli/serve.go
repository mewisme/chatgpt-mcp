package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
)

func serveCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "serve", Short: "Start the MCP server", RunE: runServer}
	addExposeFlag(cmd)
	return cmd
}

func runServer(cmd *cobra.Command, args []string) (runErr error) {
	source, err := config.Source()
	if err != nil {
		return err
	}
	if !source.Exists {
		return errors.New("chatgpt-mcp is not initialized; run chatgpt-mcp init")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := applyExposeOverride(cmd, &cfg); err != nil {
		return err
	}
	if err := config.Validate(cfg); err != nil {
		return err
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	runtimeCtx, runtimeCancel := context.WithCancel(context.WithoutCancel(cmd.Context()))
	defer runtimeCancel()

	plan, err := resolveListenerPlan(cfg.Server.Expose)
	if err != nil {
		return err
	}
	bindings, err := openHTTPBindings(cfg, plan)
	if err != nil {
		return err
	}
	defer bindings.CloseUnstarted()

	log := commandLogger(cmd)
	startedAt := time.Now().UTC()
	serviceInfo := runtimeServiceInfo(cmd)
	metadata := runtimeevent.Metadata{RunID: auth.GenerateToken("run"), PID: os.Getpid(), Managed: serviceInfo.Managed, ServiceID: serviceInfo.ID, ServiceScope: serviceInfo.Scope}
	journal, err := runtimeevent.NewJournal(config.RootPath(), runtimeevent.Options{Metadata: metadata})
	if err != nil {
		return err
	}
	recorder := runtimeevent.NewRecorder(journal, metadata)
	log.AddSink(recorder)
	runtime := app.NewWithLogger(cfg, log)
	if err := runtime.Start(runtimeCtx); err != nil {
		return err
	}
	defer func() {
		runtime.Logger.Verbose("SERVER", "server.runtime.cleanup", "Cleaning up runtime services")
		if err := runtime.Stop(); err != nil {
			runtime.Logger.Failure("SERVER", "server.runtime.cleanup.failed", "Runtime cleanup failed", err)
			if runErr == nil {
				runErr = err
			}
			return
		}
		runtime.Logger.Ready("SERVER", "server.stopped", "Server stopped")
	}()

	logReadyEndpoints(runtime.Logger, cfg, plan)

	errCh := make(chan error, max(1, len(bindings.mcpListeners)+len(bindings.adminListeners)))
	bindings.Start(runtime, errCh)
	currentCfg, currentPlan := cfg, plan
	var reloadMu sync.Mutex
	shutdownRequest := make(chan struct{}, 1)
	reload := func(_ context.Context) (runtimeReloadResult, error) {
		reloadMu.Lock()
		defer reloadMu.Unlock()
		next, err := config.Load()
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
			runtime.Logger.Warning("SERVER", "server.reload.shutdown.warning", "Previous listeners did not shut down cleanly", err)
		}
		candidate, err := openHTTPBindings(next, nextPlan)
		if err != nil {
			restored, restoreErr := restoreHTTPBindings(runtime, currentCfg, currentPlan, errCh)
			if restoreErr == nil {
				bindings = restored
			}
			return runtimeReloadResult{}, errors.Join(err, restoreErr)
		}
		if err := runtime.ReloadConfig(next); err != nil {
			candidate.CloseUnstarted()
			restored, restoreErr := restoreHTTPBindings(runtime, currentCfg, currentPlan, errCh)
			if restoreErr == nil {
				bindings = restored
			}
			return runtimeReloadResult{}, errors.Join(err, restoreErr)
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
		return runtimeStatusResult{PID: os.Getpid(), RunID: metadata.RunID, Managed: metadata.Managed, ServiceID: metadata.ServiceID, ServiceScope: metadata.ServiceScope, StartedAt: startedAt, ConfigRoot: config.RootPath(), ServerPort: currentCfg.Server.Port, AdminEnabled: currentCfg.Admin.Enabled, AdminPort: currentCfg.Admin.Port, Exposure: currentCfg.Server.Expose.Mode}
	}
	control, err := startRuntimeControl(runtimeControlOptions{RunID: metadata.RunID, Managed: metadata.Managed, ServiceID: metadata.ServiceID, ServiceScope: metadata.ServiceScope, StartedAt: startedAt, Events: recorder.Stream, Reload: reload, Status: status, Shutdown: func() {
		select {
		case shutdownRequest <- struct{}{}:
		default:
		}
	}, ClearLogs: journal.Clear})
	if err != nil {
		return errors.Join(err, bindings.Shutdown())
	}
	defer control.Close()

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
	case signalValue := <-signalCh:
		runtime.Logger.Verbose("SERVER", "server.shutdown.requested", "Shutdown requested", logger.With("signal", signalValue.String()))
		return shutdown()
	case <-shutdownRequest:
		runtime.Logger.Verbose("SERVER", "server.shutdown.requested", "Shutdown requested", logger.With("reason", "runtime control"))
		return shutdown()
	case <-cmd.Context().Done():
		runtime.Logger.Verbose("SERVER", "server.shutdown.requested", "Shutdown requested", logger.With("reason", "context canceled"))
		return shutdown()
	}
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
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
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) && first == nil {
			first = err
		}
	}
	return first
}
