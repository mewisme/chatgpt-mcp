package cli

import (
	"encoding/json"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func tunnelCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "tunnel"}
	cmd.AddCommand(tunnelStatusCommand(), tunnelConfigureCommand(), tunnelToggleCommand(true), tunnelToggleCommand(false), tunnelRunCommand())
	return cmd
}

func tunnelStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		client := tunnel.NewConfigured(withTunnelOrigin(cfg))
		status := client.Status()
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
		return nil
	}}
}

func tunnelConfigureCommand() *cobra.Command {
	var enabled bool
	var id, apiKey, command, origin, publicURL string
	var commandArgs []string
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
		if cmd.Flags().Changed("command") {
			next.Command = command
		}
		if cmd.Flags().Changed("arg") {
			next.Args = commandArgs
		}
		if cmd.Flags().Changed("origin") {
			next.Origin = origin
		}
		if cmd.Flags().Changed("public-url") {
			next.PublicURL = publicURL
		}
		cfg.Tunnel = next
		return config.Save(cfg)
	}}
	cmd.Flags().BoolVar(&enabled, "enabled", false, "enable or disable the tunnel")
	cmd.Flags().StringVar(&id, "id", "", "tunnel identifier")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "tunnel API key")
	cmd.Flags().StringVar(&command, "command", "", "tunnel process command")
	cmd.Flags().StringSliceVar(&commandArgs, "arg", nil, "tunnel process argument")
	cmd.Flags().StringVar(&origin, "origin", "", "local origin exposed by the tunnel")
	cmd.Flags().StringVar(&publicURL, "public-url", "", "public tunnel URL")
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
		return config.Save(cfg)
	}}
}

func tunnelRunCommand() *cobra.Command {
	return &cobra.Command{Use: "run", Short: "Run the configured tunnel in the foreground", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		tunnelConfig := withTunnelOrigin(cfg)
		tunnelConfig.Enabled = true
		client := tunnel.NewConfigured(tunnelConfig)
		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := client.StartContext(ctx); err != nil {
			return err
		}
		<-ctx.Done()
		return client.Stop()
	}}
}

func withTunnelOrigin(cfg config.Config) tunnel.Config {
	tunnelConfig := cfg.Tunnel
	if tunnelConfig.Origin == "" {
		tunnelConfig.Origin = config.TunnelOrigin(cfg)
	}
	return tunnelConfig
}
