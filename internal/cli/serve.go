package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/config"
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
	mcpListeners, err := listenOnHosts(plan.Hosts, cfg.Server.Port)
	if err != nil {
		return err
	}
	defer closeListeners(mcpListeners)

	var adminListeners []net.Listener
	if cfg.Admin.Enabled {
		adminListeners, err = listenOnHosts(plan.Hosts, cfg.Admin.Port)
		if err != nil {
			return err
		}
		defer closeListeners(adminListeners)
	}

	runtime := app.New(cfg)
	if err := runtime.Start(runtimeCtx); err != nil {
		return err
	}
	defer func() {
		runtime.Logger.Info("SERVER", "cleaning up runtime services")
		if err := runtime.Stop(); err != nil {
			runtime.Logger.Error("SERVER", "runtime cleanup failed", "error", err)
			if runErr == nil {
				runErr = err
			}
			return
		}
		runtime.Logger.Success("SERVER", "shutdown complete")
	}()

	logReadyEndpoints(runtime.Logger, cfg, plan)

	servers := make([]*http.Server, 0, len(mcpListeners)+len(adminListeners))
	errCh := make(chan error, len(mcpListeners)+len(adminListeners))
	for _, listener := range mcpListeners {
		server := newHTTPServer(runtime.MCPHandler())
		servers = append(servers, server)
		go serveHTTP(server, listener, errCh)
	}
	if cfg.Admin.Enabled {
		for _, listener := range adminListeners {
			server := newHTTPServer(runtime.AdminHandler())
			servers = append(servers, server)
			go serveHTTP(server, listener, errCh)
		}
	}

	shutdown := func() error {
		runtime.Logger.Info("SERVER", "stopping HTTP listeners")
		err := shutdownServers(servers)
		if err != nil {
			runtime.Logger.Error("SERVER", "HTTP shutdown failed", "error", err)
			return err
		}
		runtime.Logger.Success("SERVER", "HTTP listeners stopped")
		return nil
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runtime.Logger.Error("SERVER", "HTTP listener failed", "error", err)
			return errors.Join(err, shutdown())
		}
		return shutdown()
	case signalValue := <-signalCh:
		runtime.Logger.Warn("SERVER", "shutdown requested", "signal", signalValue.String())
		return shutdown()
	case <-cmd.Context().Done():
		runtime.Logger.Warn("SERVER", "shutdown requested", "reason", "context canceled")
		return shutdown()
	}
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
}

func serveHTTP(server *http.Server, listener net.Listener, errCh chan<- error) {
	errCh <- server.Serve(listener)
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
