package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

const (
	defaultClusterRelayListen = "127.0.0.1:37423"
	defaultClusterRelayPath   = "/cluster"
)

func clusterCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "cluster", Short: "Manage multi-runtime cluster federation"}
	cmd.AddCommand(clusterStatusCommand(), clusterRelayCommand())
	return cmd
}

func clusterRelayCommand() *cobra.Command {
	var listen, path, tokenFile string
	var allowInsecureHTTP bool
	defaults := cluster.DefaultRelayServerOptions()
	var maxConnections, maxRequestsPerSecond int
	var helloTimeout, idleTimeout, writeTimeout time.Duration
	cmd := &cobra.Command{Use: "relay", Short: "Run the cluster WebSocket relay in the foreground", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		token, err := clusterRelayToken(tokenFile)
		if err != nil {
			return err
		}
		address, err := validateClusterRelayListen(listen, allowInsecureHTTP)
		if err != nil {
			return err
		}
		path, err = normalizeClusterRelayPath(path)
		if err != nil {
			return err
		}
		options := cluster.RelayServerOptions{MaxConnections: maxConnections, MaxRequestsPerSecond: maxRequestsPerSecond, HelloTimeout: helloTimeout, IdleTimeout: idleTimeout, WriteTimeout: writeTimeout}
		if err := validateClusterRelayOptions(options); err != nil {
			return err
		}
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("listen for cluster relay: %w", err)
		}
		defer listener.Close()

		relay := cluster.NewRelayServerWithBackend(token, cluster.NewMemoryRelay(), options)
		server := &http.Server{Handler: relay.Handler(path), ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 1 << 20}
		interrupt := newForegroundInterrupt(cmd, true)
		defer interrupt.Close()
		log := commandLogger(cmd)
		defer log.Close()
		log.Ready("CLUSTER", "cluster.relay.ready", "Cluster relay is ready", logger.With("listen", listener.Addr().String()), logger.With("path", path))
		log.Detail("endpoint", "ws://"+listener.Addr().String()+path)
		log.Detail("health", "http://"+listener.Addr().String()+"/health")
		log.Detail("metrics", "http://"+listener.Addr().String()+"/metrics")
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
			_ = relay.Close()
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
	cmd.Flags().StringVar(&tokenFile, "token-file", "", "read the relay bearer token from a file instead of configured secret storage")
	cmd.Flags().BoolVar(&allowInsecureHTTP, "allow-insecure-http", false, "allow a non-loopback relay listener without TLS")
	cmd.Flags().IntVar(&maxConnections, "max-connections", defaults.MaxConnections, "maximum simultaneous relay connections")
	cmd.Flags().IntVar(&maxRequestsPerSecond, "max-requests-per-second", defaults.MaxRequestsPerSecond, "maximum requests per second per relay connection")
	cmd.Flags().DurationVar(&helloTimeout, "hello-timeout", defaults.HelloTimeout, "maximum time to receive the initial cluster hello")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", defaults.IdleTimeout, "maximum idle time between relay messages")
	cmd.Flags().DurationVar(&writeTimeout, "write-timeout", defaults.WriteTimeout, "maximum time for a relay WebSocket write")
	return cmd
}

func clusterRelayToken(tokenFile string) (string, error) {
	tokenFile = strings.TrimSpace(tokenFile)
	if tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("read cluster relay token file: %w", err)
		}
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
		return "", errors.New("cluster relay token file is empty")
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(cfg.Cluster.RelayToken)
	if token == "" {
		return "", errors.New("cluster relay token is not configured; run chatgpt-mcp config set cluster.relay_token <token> or use --token-file")
	}
	return token, nil
}

func validateClusterRelayOptions(options cluster.RelayServerOptions) error {
	if options.MaxConnections < 1 {
		return errors.New("cluster relay max connections must be greater than zero")
	}
	if options.MaxRequestsPerSecond < 1 {
		return errors.New("cluster relay max requests per second must be greater than zero")
	}
	if options.HelloTimeout <= 0 || options.IdleTimeout <= 0 || options.WriteTimeout <= 0 {
		return errors.New("cluster relay timeouts must be greater than zero")
	}
	return nil
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
	if value == "/health" || value == "/metrics" {
		return "", errors.New("cluster relay path conflicts with reserved health or metrics endpoint")
	}
	return value, nil
}

func clusterStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", Aliases: []string{"st"}, Short: "Show cluster membership, catalog compatibility, and tunnel leadership", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		status := offlineClusterStatus(cfg)
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Second)
		runtimeStatus, running, err := managedRuntimeStatus(ctx)
		cancel()
		if err != nil {
			return err
		}
		if running {
			status = runtimeStatus.Cluster
		}
		format, err := commandLogFormat(cmd)
		if err != nil {
			return err
		}
		if format == logger.FormatJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
		}
		renderClusterStatus(cmd.OutOrStdout(), status, running)
		return nil
	}}
}

func offlineClusterStatus(cfg config.Config) cluster.RuntimeStatus {
	status := cluster.RuntimeStatus{Enabled: cfg.Cluster.Enabled, RelayURL: strings.TrimSpace(cfg.Cluster.RelayURL), CatalogCompatible: true, TunnelRole: "standalone"}
	manager := workspace.NewManager(workspace.DefaultStorePath())
	if identity, err := manager.Instance(); err == nil {
		status.InstanceID, status.Name = identity.ID, identity.Name
	}
	if ids, err := manager.AdvertisedIDs(); err == nil {
		status.WorkspaceCount = len(ids)
	}
	if cfg.Cluster.Enabled {
		status.TunnelRole = "standby"
	}
	return status
}

func renderClusterStatus(out interface{ Write([]byte) (int, error) }, status cluster.RuntimeStatus, runtimeRunning bool) {
	state := "disabled"
	switch {
	case status.Enabled && status.Connected:
		state = "connected"
	case status.Enabled && runtimeRunning:
		state = "disconnected"
	case status.Enabled:
		state = "offline"
	}
	fmt.Fprintln(out, cliHeading("Cluster"))
	statusStateField(out, "status", state)
	statusField(out, "enabled", status.Enabled)
	if status.InstanceID != "" {
		statusField(out, "instance", status.InstanceID)
	}
	if status.Name != "" {
		statusField(out, "name", status.Name)
	}
	if status.RelayURL != "" {
		statusField(out, "relay", status.RelayURL)
	}
	if status.Connected {
		statusField(out, "members", fmt.Sprintf("%d online / %d known", status.OnlineMemberCount, status.MemberCount))
		statusField(out, "workspaces", status.WorkspaceCount)
		catalog := "compatible"
		if !status.CatalogCompatible {
			catalog = "incompatible"
		}
		statusField(out, "catalog", catalog)
		if status.CatalogHash != "" {
			statusField(out, "catalog hash", status.CatalogHash)
		}
	}
	if status.TunnelRole != "" {
		statusField(out, "tunnel role", strings.ReplaceAll(status.TunnelRole, "_", " "))
	}
	if status.LeaderInstanceID != "" {
		statusField(out, "leader", status.LeaderInstanceID)
		statusField(out, "epoch", status.LeaderEpoch)
	}
	if status.CatalogError != "" {
		statusField(out, "catalog error", status.CatalogError)
	}
	if status.LastError != "" {
		statusField(out, "error", status.LastError)
	}
}
