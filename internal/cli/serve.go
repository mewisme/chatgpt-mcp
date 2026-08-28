package cli

import (
	"context"
	"errors"
	"fmt"
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
	return &cobra.Command{Use: "serve", Short: "Start the MCP server", RunE: runServer}
}

func runServer(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(config.Path()); err != nil {
		if os.IsNotExist(err) {
			return errors.New("chatgpt-mcp is not initialized; run chatgpt-mcp init")
		}
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := config.Validate(cfg); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mcpAddr := net.JoinHostPort(cfg.Server.Host, fmt.Sprint(cfg.Server.Port))
	mcpListener, err := net.Listen("tcp", mcpAddr)
	if err != nil {
		return err
	}
	defer mcpListener.Close()

	var adminListener net.Listener
	adminAddr := ""
	if cfg.Admin.Enabled {
		adminAddr = net.JoinHostPort(cfg.Server.Host, fmt.Sprint(cfg.Admin.Port))
		adminListener, err = net.Listen("tcp", adminAddr)
		if err != nil {
			return err
		}
		defer adminListener.Close()
	}

	runtime := app.New(cfg)
	if err := runtime.Start(ctx); err != nil {
		return err
	}
	defer runtime.Stop()

	runtime.Logger.Info("MCP", "endpoint ready", "url", "http://"+mcpAddr+"/mcp")
	if cfg.Admin.Enabled {
		runtime.Logger.Info("ADMIN", "dashboard ready", "url", "http://"+adminAddr+"/")
	}

	mcpServer := newHTTPServer(runtime.MCPHandler())
	servers := []*http.Server{mcpServer}
	errCh := make(chan error, 2)
	go serveHTTP(mcpServer, mcpListener, errCh)
	if cfg.Admin.Enabled {
		adminServer := newHTTPServer(runtime.AdminHandler())
		servers = append(servers, adminServer)
		go serveHTTP(adminServer, adminListener, errCh)
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdownServers(servers)
			return err
		}
		return nil
	case <-ctx.Done():
		return shutdownServers(servers)
	}
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
}

func serveHTTP(server *http.Server, listener net.Listener, errCh chan<- error) {
	errCh <- server.Serve(listener)
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
