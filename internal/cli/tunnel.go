package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/telemetry"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func tunnelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "tunnel",
		Short:             "Manage OpenAI Secure MCP Tunnel instances and tunnel providers",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeTunnelDispatch,
		RunE:              runTunnelProviderDispatch,
	}
	cmd.AddCommand(tunnelLocalListCommand(), tunnelCollectionStatusCommand(), tunnelAddCommand(), tunnelAttachCommand(), tunnelUpdateCommand(), tunnelDetachCommand(), tunnelLocalToggleCommand(true), tunnelLocalToggleCommand(false), tunnelStartCommand(), tunnelStopCommand(), tunnelProviderCommand("cf", "CF Tunnel"), tunnelManagedCommand(), tunnelAdminProfilesCommand(), tunnelRunCommand())
	seen := map[string]struct{}{"cf": {}}
	if refs, err := application.ListTunnelProviders(); err == nil {
		for _, ref := range refs {
			if _, ok := seen[ref.Provider]; ok || reservedTunnelCommand(ref.Provider) {
				continue
			}
			seen[ref.Provider] = struct{}{}
			cmd.AddCommand(tunnelProviderCommand(ref.Provider, ref.Name))
		}
	}
	return cmd
}

func normalizeTunnelIDs(values []string) []string { return application.NormalizeTunnelIDs(values) }

func tunnelRunCommand() *cobra.Command {
	return &cobra.Command{Use: "run <id>", Short: "Run one attached OpenAI Secure MCP Tunnel in the foreground", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		logCommandStep(cmd, "TUNNEL", "tunnel.runtime.loading", "Loading tunnel runtime configuration")
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load tunnel runtime configuration: %w", err)
		}
		id := strings.TrimSpace(args[0])
		var tunnelConfig tunnel.Config
		for _, instance := range cfg.RuntimeTunnels().Instances {
			if instance.ID == id {
				tunnelConfig = tunnel.Config{Enabled: true, ID: instance.ID, APIKey: instance.APIKey, ControlPlaneBaseURL: instance.ControlPlaneBaseURL, OrganizationID: instance.OrganizationID}
				break
			}
		}
		if tunnelConfig.ID == "" {
			return fmt.Errorf("tunnel %q is not attached", id)
		}
		if err := tunnel.ValidateConfig(tunnelConfig); err != nil {
			return err
		}

		log := commandLogger(cmd)
		logCommandStep(cmd, "TUNNEL", "tunnel.tools.initializing", "Initializing MCP tool runtime")
		runtime := tools.NewRuntimeWithAccess(cfg.Permissions.AllowDirs, func() (bool, int) { return cfg.Admin.Enabled, cfg.Admin.Port })
		if err := runtime.SetShellExecutable(cfg.Shell.Executable); err != nil {
			return err
		}
		runtime.SetShellPath(cfg.Shell.Path)
		if err := runtime.Workspaces.Activate(); err != nil {
			return err
		}
		defer func() {
			if err := runtime.Workspaces.Deactivate(); err != nil && runErr == nil {
				runErr = err
			}
		}()
		telemetry.AttachTools(runtime, nil, log)
		runtimeCtx, runtimeCancel := context.WithCancel(context.WithoutCancel(cmd.Context()))
		defer runtimeCancel()
		interrupt := newForegroundInterrupt(cmd, true)
		defer interrupt.Close()
		shutdownCtx := interrupt.Context

		client := tunnel.NewConfiguredWithLogger(tunnelConfig, runtime, log)
		if metadata, err := config.LoadTunnelMetadata(tunnelConfig.ID); err == nil {
			if seedErr := client.SeedMetadata(metadata); seedErr != nil {
				logCommandDebug(cmd, "TUNNEL", "tunnel.metadata.seed-failed", "Cached tunnel metadata could not be seeded", logger.WithDebug("error", seedErr.Error()))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			logCommandDebug(cmd, "TUNNEL", "tunnel.metadata.load-failed", "Cached tunnel metadata could not be loaded", logger.WithDebug("error", err.Error()))
		}
		client.SetLifecycleObserver(func(event tunnel.LifecycleEvent) { logTunnelLifecycle(log, event) })
		logCommandStep(cmd, "TUNNEL", "tunnel.runtime.starting", "Starting tunnel runtime", logger.WithVerbose("tunnel_id", tunnelConfig.ID))
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
