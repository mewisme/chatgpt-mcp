package cli

import (
	"context"
	"encoding/json"
	"os/signal"
	"syscall"
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
	cmd.AddCommand(tunnelStatusCommand(), tunnelConfigureCommand(), tunnelToggleCommand(true), tunnelToggleCommand(false), tunnelRunCommand())
	return cmd
}

func tunnelStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(tunnel.NewConfigured(cfg.Tunnel, nil).Status(), "", "  ")
		cmd.Println(string(data))
		return nil
	}}
}

func tunnelConfigureCommand() *cobra.Command {
	var enabled bool
	var id, apiKey, controlPlaneBaseURL, organizationID string
	cmd := &cobra.Command{Use: "configure", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		next := cfg.Tunnel
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
		if err := config.Save(cfg); err != nil {
			return err
		}
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Success("TUNNEL", "OpenAI Secure MCP Tunnel configuration saved")
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
	use := "disable"
	if enabled {
		use = "enable"
	}
	return &cobra.Command{Use: use, RunE: func(cmd *cobra.Command, args []string) error {
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
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Success("TUNNEL", "OpenAI Secure MCP Tunnel "+state)
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

		log := logger.NewCLIWithWriter(cmd.OutOrStdout())
		runtime := tools.NewRuntimeWithFeatures(cfg.Features)
		telemetry.AttachTools(runtime, nil, log)
		runtimeCtx, runtimeCancel := context.WithCancel(context.WithoutCancel(cmd.Context()))
		defer runtimeCancel()
		shutdownCtx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		client := tunnel.NewConfiguredWithLogger(tunnelConfig, runtime, log)
		if err := client.StartContext(runtimeCtx); err != nil {
			return err
		}
		defer func() {
			status := client.Status()
			if status.Running {
				log.Info("TUNNEL", "stopping", "tunnel_id", tunnelConfig.ID)
				if err := client.Stop(); err != nil {
					log.Error("TUNNEL", "failed to stop", "error", err)
					if runErr == nil {
						runErr = err
					}
				} else {
					log.Success("TUNNEL", "stopped", "tunnel_id", tunnelConfig.ID)
				}
			}
			if runtime.Upstream != nil {
				log.Info("UPSTREAM", "stopping upstream servers")
				upstreamCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := runtime.Upstream.Shutdown(upstreamCtx)
				cancel()
				if err != nil {
					log.Error("UPSTREAM", "shutdown failed", "error", err)
					if runErr == nil {
						runErr = err
					}
				} else {
					log.Success("UPSTREAM", "stopped")
				}
			}
			if runErr == nil {
				log.Success("TUNNEL", "shutdown complete")
			}
		}()

		log.Info("TUNNEL", "connecting to OpenAI Secure MCP Tunnel", "tunnel_id", tunnelConfig.ID)
		if err := client.WaitUntilReady(shutdownCtx); err != nil {
			if shutdownCtx.Err() != nil {
				log.Warn("TUNNEL", "shutdown requested")
				return nil
			}
			return err
		}
		log.Success("TUNNEL", "connected", "tunnel_id", tunnelConfig.ID)
		<-shutdownCtx.Done()
		log.Warn("TUNNEL", "shutdown requested")
		return nil
	}}
}
