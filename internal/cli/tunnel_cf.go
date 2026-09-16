package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func tunnelCFCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "cf", Short: "Expose local MCP and Admin HTTP through Cloudflare Quick Tunnels", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(tunnelCFStatusCommand(), tunnelCFStartCommand(), tunnelCFStopCommand())
	return cmd
}

func tunnelCFStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status [mcp|admin|all]", Short: "Show Cloudflare Quick Tunnel status", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		target := "all"
		if len(args) == 1 {
			parsed, err := cftunnelplugin.ParseTarget(args[0])
			if err != nil {
				return err
			}
			target = parsed
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		var live *runtimecontrol.CFTunnelStatus
		status, err := requestRuntimeStatus(cmd.Context())
		if err == nil {
			live = status.CFTunnel
		} else if !runtimecontrol.IsUnavailable(err) {
			return err
		}
		renderCFTunnelStatus(cmd.OutOrStdout(), cfg, live, target)
		return nil
	}}
}

func tunnelCFStartCommand() *cobra.Command {
	return &cobra.Command{Use: "start <mcp|admin|all>", Short: "Start a Cloudflare Quick Tunnel for MCP and/or Admin HTTP", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := application.StartCFTunnel(cmd.Context(), cfg, args[0]); err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "CF Tunnel start requested", "target", strings.ToLower(strings.TrimSpace(args[0])))
		return nil
	}}
}

func tunnelCFStopCommand() *cobra.Command {
	return &cobra.Command{Use: "stop <mcp|admin|all>", Short: "Stop a Cloudflare Quick Tunnel", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.StopCFTunnel(cmd.Context(), args[0]); err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "CF Tunnel stop requested", "target", strings.ToLower(strings.TrimSpace(args[0])))
		return nil
	}}
}

func renderCFTunnelStatus(out io.Writer, cfg config.Config, live *runtimecontrol.CFTunnelStatus, target string) {
	fmt.Fprintln(out, "CF Tunnel  ephemeral Quick Tunnels, not Secure MCP")
	for _, item := range cfTunnelStatusItems(cfg, live) {
		if target != "all" && item.Target != target {
			continue
		}
		fmt.Fprintf(out, "  %-5s  %s\n", item.Target, cfTunnelStatusLine(item))
	}
}

func cfTunnelStatusItems(cfg config.Config, live *runtimecontrol.CFTunnelStatus) []cftunnelplugin.TargetStatus {
	if live != nil {
		items := make([]cftunnelplugin.TargetStatus, 0, len(live.Targets))
		for _, item := range live.Targets {
			items = append(items, cftunnelplugin.TargetStatus{
				Target: item.Target, Desired: item.Desired, Running: item.Running, Ready: item.Ready,
				URL: item.URL, Origin: item.Origin, LastError: item.LastError,
			})
		}
		if len(items) > 0 {
			return items
		}
	}
	snap := application.CFTunnelSnapshot(cfg)
	return []cftunnelplugin.TargetStatus{
		cfTunnelSnapshotItem(cftunnelplugin.TargetMCP, snap.PluginEnabled && snap.DesiredMCP, snap.MCP),
		cfTunnelSnapshotItem(cftunnelplugin.TargetAdmin, snap.PluginEnabled && snap.DesiredAdmin, snap.Admin),
	}
}

func cfTunnelSnapshotItem(target string, desired bool, endpoint cftunnelplugin.Endpoint) cftunnelplugin.TargetStatus {
	item := cftunnelplugin.TargetStatus{Target: target, Desired: desired}
	if endpoint.AuthErr != nil && desired {
		item.LastError = endpoint.AuthErr.Error()
	}
	return item
}

func cfTunnelStatusLine(item cftunnelplugin.TargetStatus) string {
	if item.LastError != "" {
		return "degraded · " + item.LastError
	}
	if item.Ready && item.URL != "" {
		return item.URL + " · ephemeral"
	}
	if item.Running {
		return "connecting"
	}
	if item.Desired {
		return "offline"
	}
	return "disabled"
}
