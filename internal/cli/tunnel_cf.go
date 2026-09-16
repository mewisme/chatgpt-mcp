package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/pluginhost"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func reservedTunnelCommand(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "list", "status", "add", "attach", "update", "detach", "enable", "disable", "start", "stop", "run", "managed", "admin", "help":
		return true
	default:
		return false
	}
}

func tunnelProviderCommand(provider, name string) *cobra.Command {
	if strings.TrimSpace(name) == "" {
		name = provider
	}
	cmd := &cobra.Command{Use: provider, Short: "Expose local MCP and Admin HTTP through " + name, Args: cobra.NoArgs, ValidArgsFunction: completeTunnelProviderAction, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(tunnelProviderStatusCommand(provider), tunnelProviderStartCommand(provider), tunnelProviderStopCommand(provider))
	return cmd
}

func tunnelProviderStatusCommand(provider string) *cobra.Command {
	return &cobra.Command{Use: "status [mcp|admin|all]", Short: "Show tunnel provider status", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeStatic("mcp", "admin", "all"), RunE: func(cmd *cobra.Command, args []string) error {
		target := "all"
		if len(args) == 1 {
			target = args[0]
		}
		return runTunnelProviderStatus(cmd, provider, target)
	}}
}

func tunnelProviderStartCommand(provider string) *cobra.Command {
	return &cobra.Command{Use: "start <mcp|admin|all>", Short: "Start a tunnel provider target", Args: cobra.ExactArgs(1), ValidArgsFunction: completeStatic("mcp", "admin", "all"), RunE: func(cmd *cobra.Command, args []string) error {
		return runTunnelProviderStart(cmd, provider, args[0])
	}}
}

func tunnelProviderStopCommand(provider string) *cobra.Command {
	return &cobra.Command{Use: "stop <mcp|admin|all>", Short: "Stop a tunnel provider target", Args: cobra.ExactArgs(1), ValidArgsFunction: completeStatic("mcp", "admin", "all"), RunE: func(cmd *cobra.Command, args []string) error {
		return runTunnelProviderStop(cmd, provider, args[0])
	}}
}

func runTunnelProviderDispatch(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	provider := args[0]
	if reservedTunnelCommand(provider) {
		return fmt.Errorf("unknown command %q for %q", provider, cmd.CommandPath())
	}
	if len(args) == 1 {
		if _, err := application.LookupTunnelProvider(provider); err != nil {
			return err
		}
		return fmt.Errorf("usage: cgm tunnel %s status|start|stop", provider)
	}
	return runTunnelProviderAction(cmd, provider, args[1], args[2:])
}

func runTunnelProviderAction(cmd *cobra.Command, provider, action string, rest []string) error {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "status":
		target := "all"
		if len(rest) > 0 {
			target = rest[0]
		}
		return runTunnelProviderStatus(cmd, provider, target)
	case "start":
		if len(rest) != 1 {
			return fmt.Errorf("usage: cgm tunnel %s start <mcp|admin|all>", provider)
		}
		return runTunnelProviderStart(cmd, provider, rest[0])
	case "stop":
		if len(rest) != 1 {
			return fmt.Errorf("usage: cgm tunnel %s stop <mcp|admin|all>", provider)
		}
		return runTunnelProviderStop(cmd, provider, rest[0])
	default:
		return fmt.Errorf("unknown tunnel provider action %q", action)
	}
}

func runTunnelProviderStatus(cmd *cobra.Command, provider, target string) error {
	if _, err := application.LookupTunnelProvider(provider); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var runtime runtimecontrol.RuntimeStatus
	status, err := requestRuntimeStatus(cmd.Context())
	if err == nil {
		runtime = status
	} else if !runtimecontrol.IsUnavailable(err) {
		return err
	}
	item, ok := tunnelProviderFromRuntime(cfg, runtime, provider)
	if !ok {
		configured, confErr := application.ConfiguredTunnelProviderStatus(cfg, provider)
		if confErr != nil {
			return confErr
		}
		item = configured
	}
	renderTunnelProviderStatus(cmd.OutOrStdout(), item, target)
	return nil
}

func runTunnelProviderStart(cmd *cobra.Command, provider, target string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := application.StartTunnelProvider(cmd.Context(), cfg, provider, target); err != nil {
		return err
	}
	commandLogger(cmd).Success("TUNNEL", "Tunnel provider start requested", "provider", provider, "target", strings.ToLower(strings.TrimSpace(target)))
	return nil
}

func runTunnelProviderStop(cmd *cobra.Command, provider, target string) error {
	if err := application.StopTunnelProvider(cmd.Context(), provider, target); err != nil {
		return err
	}
	commandLogger(cmd).Success("TUNNEL", "Tunnel provider stop requested", "provider", provider, "target", strings.ToLower(strings.TrimSpace(target)))
	return nil
}

func tunnelProviderFromRuntime(cfg config.Config, runtime runtimecontrol.RuntimeStatus, provider string) (runtimecontrol.TunnelProviderStatus, bool) {
	for _, item := range application.MergeTunnelProviderStatus(cfg, runtime) {
		if item.Provider == provider {
			return item, true
		}
	}
	return runtimecontrol.TunnelProviderStatus{}, false
}

func renderTunnelProviderStatus(out io.Writer, item runtimecontrol.TunnelProviderStatus, target string) {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = item.Provider
	}
	fmt.Fprintf(out, "%s  ephemeral Quick Tunnels, not Secure MCP\n", name)
	for _, targetItem := range item.Targets {
		if target != "all" && targetItem.Target != target {
			continue
		}
		fmt.Fprintf(out, "  %-5s  %s\n", targetItem.Target, targetItem.Line())
	}
}

func renderCFTunnelStatus(out io.Writer, cfg config.Config, live *runtimecontrol.CFTunnelStatus, target string) {
	runtime := runtimecontrol.RuntimeStatus{CFTunnel: live}
	item, ok := tunnelProviderFromRuntime(cfg, runtime, "cf")
	if !ok {
		item = live.AsProvider()
	}
	renderTunnelProviderStatus(out, item, target)
}

func watchCFTunnel(log *logger.Logger) {
	pluginhost.SetRuntimeObserver(func(event cftunnelplugin.LifecycleEvent) {
		logTunnelProviderLifecycle(log, "cf", string(event.State), event.Target, event.URL, event.Error)
	})
}

func logCFTunnelLifecycle(log *logger.Logger, event cftunnelplugin.LifecycleEvent) {
	logTunnelProviderLifecycle(log, "cf", string(event.State), event.Target, event.URL, event.Error)
}

func logTunnelProviderLifecycle(log *logger.Logger, provider, state, target, url, errText string) {
	if log == nil {
		return
	}
	name := provider + " tunnel"
	if provider == "cf" {
		name = "CF Tunnel"
	}
	fields := []logger.Field{logger.WithVerbose("target", target)}
	if url != "" {
		fields = append(fields, logger.WithVerbose("url", url))
	}
	switch state {
	case "connecting":
		log.Action("TUNNEL", provider+"-tunnel.connecting", "Connecting "+name, fields...)
	case "reconnecting":
		log.Action("TUNNEL", provider+"-tunnel.reconnecting", "Reconnecting "+name, fields...)
	case "ready":
		log.Ready("TUNNEL", provider+"-tunnel.ready", name+" ready", fields...)
	case "degraded":
		var eventErr error
		if errText != "" {
			eventErr = errors.New(errText)
		}
		log.Warning("TUNNEL", provider+"-tunnel.degraded", name+" degraded", eventErr, fields...)
	case "stopped":
		log.Ready("TUNNEL", provider+"-tunnel.stopped", name+" stopped", fields...)
	}
}
