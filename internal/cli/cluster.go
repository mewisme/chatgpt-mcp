package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

const (
	defaultClusterRelayListen = "127.0.0.1:37423"
	defaultClusterRelayPath   = "/cluster"
)

func clusterCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "cluster", Short: "Manage multi-runtime cluster federation"}
	cmd.AddCommand(clusterRelayCommand())
	return cmd
}

func clusterRelayCommand() *cobra.Command {
	var listen, path string
	var allowInsecureHTTP bool
	cmd := &cobra.Command{Use: "relay", Short: "Run the cluster WebSocket relay in the foreground", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		token := strings.TrimSpace(cfg.Cluster.RelayToken)
		if token == "" {
			return errors.New("cluster relay token is not configured; run chatgpt-mcp config set cluster.relay_token <token>")
		}
		address, err := validateClusterRelayListen(listen, allowInsecureHTTP)
		if err != nil {
			return err
		}
		path, err = normalizeClusterRelayPath(path)
		if err != nil {
			return err
		}
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("listen for cluster relay: %w", err)
		}
		defer listener.Close()

		mux := http.NewServeMux()
		mux.Handle(path, cluster.NewRelayServer(token))
		server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
		interrupt := newForegroundInterrupt(cmd, true)
		defer interrupt.Close()
		log := commandLogger(cmd)
		defer log.Close()
		log.Ready("CLUSTER", "cluster.relay.ready", "Cluster relay is ready", logger.With("listen", listener.Addr().String()), logger.With("path", path))
		log.Detail("endpoint", "ws://"+listener.Addr().String()+path)
		if !isLoopbackListen(address) {
			log.Warning("CLUSTER", "cluster.relay.insecure", "Cluster relay is exposed over insecure HTTP", errors.New("terminate TLS before exposing this endpoint remotely"))
		}

		errCh := make(chan error, 1)
		go func() {
			err := server.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
			errCh <- nil
		}()
		select {
		case err := <-errCh:
			if err != nil {
				return fmt.Errorf("serve cluster relay: %w", err)
			}
			return nil
		case <-interrupt.Context.Done():
			log.Action("CLUSTER", "cluster.relay.stopping", "Stopping cluster relay")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := server.Shutdown(ctx)
			cancel()
			if err != nil {
				return fmt.Errorf("stop cluster relay: %w", err)
			}
			log.Ready("CLUSTER", "cluster.relay.stopped", "Cluster relay stopped")
			return nil
		}
	}}
	cmd.Flags().StringVar(&listen, "listen", defaultClusterRelayListen, "relay listen address")
	cmd.Flags().StringVar(&path, "path", defaultClusterRelayPath, "WebSocket relay path")
	cmd.Flags().BoolVar(&allowInsecureHTTP, "allow-insecure-http", false, "allow a non-loopback relay listener without TLS")
	return cmd
}

func validateClusterRelayListen(value string, allowInsecure bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("cluster relay listen address is required")
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("invalid cluster relay listen address %q: %w", value, err)
	}
	if strings.TrimSpace(host) == "" {
		return "", errors.New("cluster relay listen host must be explicit")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("cluster relay port must be between 1 and 65535: %q", portText)
	}
	if !allowInsecure && !isLoopbackHost(host) {
		return "", errors.New("non-loopback cluster relay requires --allow-insecure-http; prefer a TLS reverse proxy and wss:// for remote runtimes")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func isLoopbackListen(value string) bool {
	host, _, err := net.SplitHostPort(value)
	return err == nil && isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeClusterRelayPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("cluster relay path is required")
	}
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") {
		return "", errors.New("cluster relay path must start with / and must not contain a query or fragment")
	}
	if value != "/" {
		value = strings.TrimRight(value, "/")
	}
	return value, nil
}
