package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/telemetry"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func tunnelCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "tunnel", Short: "Manage the builtin OpenAI Secure MCP Tunnel"}
	cmd.AddCommand(tunnelAdminCommand(), tunnelListCommand(), tunnelGetCommand(), tunnelCreateCommand(), tunnelUpdateCommand(), tunnelDeleteCommand(), tunnelStatusCommand(), tunnelSyncCommand(), tunnelConfigureCommand(), tunnelToggleCommand(true), tunnelToggleCommand(false), tunnelRunCommand())
	return cmd
}

func normalizeTunnelIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func tunnelStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", Aliases: []string{"st"}, Short: "Show tunnel configuration, runtime state, and metadata", Args: cobra.NoArgs, RunE: runTunnelStatus}
}

func runTunnelStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	status := fetchTunnelStatus(cmd.Context(), cfg.Tunnel)
	runtimeCtx, cancel := context.WithTimeout(cmd.Context(), time.Second)
	runtimeStatus, runtimeRunning, err := managedRuntimeStatus(runtimeCtx)
	cancel()
	if err != nil {
		return err
	}
	if runtimeRunning {
		status.Running = runtimeStatus.TunnelRunning
		status.Ready = runtimeStatus.TunnelReady
		status.Restarting = runtimeStatus.TunnelRestarting
		status.LastError = runtimeStatus.TunnelLastError
		if status.ID == "" {
			status.ID = runtimeStatus.TunnelID
		}
	}
	format, err := commandLogFormat(cmd)
	if err != nil {
		return err
	}
	if format == logger.FormatJSON {
		data, err := json.Marshal(status)
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	}
	if runtimeRunning && transientTunnelState(tunnelCLIState(cfg.Tunnel, status, true)) && logger.CanAnimate(cmd.OutOrStdout()) {
		runtimeStatus = animateRuntimeTunnelState(cmd, runtimeStatus, statusTunnelWatchTimeout)
		status.Running = runtimeStatus.TunnelRunning
		status.Ready = runtimeStatus.TunnelReady
		status.Restarting = runtimeStatus.TunnelRestarting
		status.LastError = runtimeStatus.TunnelLastError
	}
	verbose, _ := commandLogMode(cmd)
	renderTunnelStatusText(cmd.OutOrStdout(), cfg.Tunnel, status, runtimeRunning, verbose)
	return nil
}

func renderTunnelStatusText(out io.Writer, cfg tunnel.Config, status tunnel.Status, runtimeRunning, verbose bool) {
	state := tunnelCLIState(cfg, status, runtimeRunning)
	renderTunnelStateLine(out, state)
	fmt.Fprintln(out, "\n"+cliHeading("Tunnel"))
	statusStateField(out, "status", state)
	statusField(out, "enabled", status.Enabled)
	statusField(out, "configured", tunnel.Configured(cfg))
	if status.ID != "" {
		statusField(out, "id", status.ID)
	}
	if status.Metadata != nil {
		metadata := status.Metadata
		if metadata.Name != "" {
			statusField(out, "name", metadata.Name)
		}
		if verbose {
			if metadata.Description != "" {
				statusField(out, "description", metadata.Description)
			}
			if metadata.Creator != "" {
				statusField(out, "creator", metadata.Creator)
			}
			if len(metadata.WorkspaceIDs) > 0 {
				statusField(out, "workspaces", strings.Join(metadata.WorkspaceIDs, ", "))
			}
			if len(metadata.OrganizationIDs) > 0 {
				statusField(out, "organizations", strings.Join(metadata.OrganizationIDs, ", "))
			}
			if len(metadata.TenantIDs) > 0 {
				statusField(out, "tenants", strings.Join(metadata.TenantIDs, ", "))
			}
		}
	}
	if status.AdminKeyConfigured && status.AdminScope != nil {
		statusField(out, "admin", "configured · "+formatTunnelAdminScope(*status.AdminScope))
	} else if verbose {
		statusField(out, "admin", "not configured")
	}
	if verbose {
		controlPlane := status.ControlPlaneBaseURL
		if strings.TrimSpace(controlPlane) == "" {
			controlPlane = "default"
		}
		statusField(out, "control plane", controlPlane)
		if !status.StartedAt.IsZero() {
			statusField(out, "started", status.StartedAt.Local().Format(time.RFC3339))
		}
		if status.MetadataError != "" {
			statusField(out, "metadata", "unavailable: "+status.MetadataError)
		}
		if status.LastError != "" {
			statusField(out, "error", status.LastError)
		}
	}
}

func tunnelCLIState(cfg tunnel.Config, status tunnel.Status, runtimeRunning bool) string {
	if !status.Enabled {
		return "disabled"
	}
	if !tunnel.Configured(cfg) {
		return "not configured"
	}
	if !runtimeRunning {
		return "offline"
	}
	switch {
	case status.Ready:
		return "connected"
	case status.Restarting:
		return "reconnecting"
	case status.Running:
		return "connecting"
	case status.LastError != "":
		return "failed"
	default:
		return "starting"
	}
}

func fetchTunnelStatus(_ context.Context, cfg tunnel.Config) tunnel.Status {
	client := tunnel.NewConfigured(cfg, nil)
	if metadata, err := config.LoadTunnelMetadata(cfg.ID); err == nil {
		_ = client.SeedMetadata(metadata)
	}
	return client.Status()
}

func tunnelSyncCommand() *cobra.Command {
	return &cobra.Command{Use: "sync", Short: "Fetch and persist metadata for the configured tunnel", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !tunnel.Configured(cfg.Tunnel) {
			return errors.New("configured tunnel id and runtime API key are required")
		}
		log := commandLogger(cmd)
		defer log.Close()
		log.Action("TUNNEL", "tunnel.metadata.syncing", "Syncing tunnel metadata")
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		metadata, path, err := config.SyncTunnelMetadata(ctx, cfg.Tunnel)
		if err != nil {
			return err
		}
		log.Success("TUNNEL", "Tunnel metadata synced")
		log.Detail("id", metadata.ID)
		if metadata.Name != "" {
			log.Detail("name", metadata.Name)
		}
		log.Detail("metadata", path)
		return nil
	}}
}

func tunnelConfigureCommand() *cobra.Command {
	var enabled bool
	var id, apiKey, controlPlaneBaseURL, organizationID string
	cmd := &cobra.Command{Use: "configure", Short: "Configure the builtin OpenAI Secure MCP Tunnel", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		previous := cfg.Tunnel
		next := previous
		if cmd.Flags().Changed("enabled") {
			next.Enabled = enabled
		}
		if cmd.Flags().Changed("id") {
			next.ID = id
		}
		if cmd.Flags().Changed("api-key") {
			next.APIKey = apiKey
		}
		if cmd.Flags().Changed("control-plane-base-url") {
			next.ControlPlaneBaseURL = controlPlaneBaseURL
		}
		if cmd.Flags().Changed("organization-id") {
			next.OrganizationID = organizationID
		}
		cfg.Tunnel = next
		if err := config.Validate(cfg); err != nil {
			return err
		}
		if tunnel.Configured(next) && (previous.ID != next.ID || previous.APIKey != next.APIKey || previous.ControlPlaneBaseURL != next.ControlPlaneBaseURL) {
			ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
			_, _, metadataErr := config.SyncTunnelMetadata(ctx, next)
			cancel()
			if metadataErr != nil {
				return fmt.Errorf("persist tunnel metadata: %w", metadataErr)
			}
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "OpenAI Secure MCP Tunnel configuration saved")
		return nil
	}}
	cmd.Flags().BoolVar(&enabled, "enabled", false, "enable or disable the OpenAI Secure MCP Tunnel")
	cmd.Flags().StringVar(&id, "id", "", "OpenAI tunnel identifier")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "OpenAI tunnel runtime API key")
	cmd.Flags().StringVar(&controlPlaneBaseURL, "control-plane-base-url", "", "OpenAI tunnel control plane base URL")
	cmd.Flags().StringVar(&organizationID, "organization-id", "", "OpenAI organization identifier")
	return cmd
}

func tunnelToggleCommand(enabled bool) *cobra.Command {
	use, short := "disable", "Disable the builtin OpenAI Secure MCP Tunnel"
	if enabled {
		use, short = "enable", "Enable the builtin OpenAI Secure MCP Tunnel"
	}
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.Tunnel.Enabled = enabled
		if err := config.Validate(cfg); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		state := "disabled"
		if enabled {
			state = "enabled"
		}
		commandLogger(cmd).Success("TUNNEL", "OpenAI Secure MCP Tunnel "+state)
		return nil
	}}
}

func tunnelRunCommand() *cobra.Command {
	return &cobra.Command{Use: "run", Short: "Run the builtin OpenAI Secure MCP Tunnel in the foreground", RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		tunnelConfig := cfg.Tunnel
		tunnelConfig.Enabled = true
		if err := tunnel.ValidateConfig(tunnelConfig); err != nil {
			return err
		}

		log := commandLogger(cmd)
		defer log.Close()
		runtime := tools.NewRuntimeWithAccess(cfg.Features, cfg.Permissions.AllowDirs)
		telemetry.AttachTools(runtime, nil, log)
		runtimeCtx, runtimeCancel := context.WithCancel(context.WithoutCancel(cmd.Context()))
		defer runtimeCancel()
		interrupt := newForegroundInterrupt(cmd, true)
		defer interrupt.Close()
		shutdownCtx := interrupt.Context

		client := tunnel.NewConfiguredWithLogger(tunnelConfig, runtime, log)
		if metadata, err := config.LoadTunnelMetadata(tunnelConfig.ID); err == nil {
			_ = client.SeedMetadata(metadata)
		}
		client.SetLifecycleObserver(func(event tunnel.LifecycleEvent) { logTunnelLifecycle(log, event) })
		if err := client.StartContext(runtimeCtx); err != nil {
			return err
		}
		defer func() {
			status := client.Status()
			if status.Running || status.Restarting {
				log.Action("TUNNEL", "tunnel.stopping", "Stopping tunnel", logger.WithVerbose("tunnel_id", tunnelConfig.ID))
				if err := client.Stop(); err != nil {
					log.Failure("TUNNEL", "tunnel.stop.failed", "Failed to stop tunnel", err)
					if runErr == nil {
						runErr = err
					}
				}
			}
			if runtime.Upstream != nil {
				log.Verbose("UPSTREAM", "upstream.stopping", "Stopping upstream servers")
				upstreamCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := runtime.Upstream.Shutdown(upstreamCtx)
				cancel()
				if err != nil {
					log.Failure("UPSTREAM", "upstream.shutdown.failed", "Upstream shutdown failed", err)
					if runErr == nil {
						runErr = err
					}
				} else {
					log.Verbose("UPSTREAM", "upstream.stopped", "Upstream servers stopped")
				}
			}
			if runErr == nil {
				log.Verbose("TUNNEL", "tunnel.shutdown.complete", "Tunnel shutdown complete")
			}
		}()

		if err := client.WaitUntilReady(shutdownCtx); err != nil {
			if shutdownCtx.Err() != nil {
				log.Verbose("TUNNEL", "tunnel.shutdown.requested", "Shutdown requested")
				return nil
			}
			return err
		}
		<-shutdownCtx.Done()
		log.Verbose("TUNNEL", "tunnel.shutdown.requested", "Shutdown requested", logger.With("reason", interrupt.Reason()))
		return nil
	}}
}

func logTunnelLifecycle(log *logger.Logger, event tunnel.LifecycleEvent) {
	fields := []logger.Field{}
	if event.ID != "" {
		fields = append(fields, logger.WithVerbose("tunnel_id", event.ID))
	}
	switch event.State {
	case tunnel.LifecycleConnecting:
		log.Action("TUNNEL", "tunnel.connecting", "Connecting tunnel", fields...)
	case tunnel.LifecycleReconnecting:
		fields = append(fields, logger.WithVerbose("attempt", event.Attempt), logger.WithVerbose("retry_in", event.RetryIn.String()))
		log.Action("TUNNEL", "tunnel.reconnecting", "Reconnecting tunnel", fields...)
	case tunnel.LifecycleReady:
		log.Ready("TUNNEL", "tunnel.connected", "Tunnel connected", fields...)
	case tunnel.LifecycleDegraded:
		var eventErr error
		if event.Message != "" {
			eventErr = errors.New(event.Message)
		}
		log.Warning("TUNNEL", "tunnel.degraded", "Tunnel degraded", eventErr, fields...)
	case tunnel.LifecycleStopped:
		log.Ready("TUNNEL", "tunnel.stopped", "Tunnel stopped", fields...)
	}
}
