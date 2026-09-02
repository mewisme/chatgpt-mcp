package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	mcpnetwork "go.mewis.me/chatgpt-mcp/internal/network"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type statusSnapshot struct {
	Source        configformat.Source
	Config        config.Config
	Runtime       runtimeStatusResult
	Running       bool
	Workspaces    int
	Upstreams     int
	Services      []installedManagedService
	ListenerPlan  listenerPlan
	ListenerError error
	Tunnel        tunnel.Status
}

const statusTunnelWatchTimeout = 35 * time.Second

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "Show runtime health and local configuration",
		Args:    cobra.NoArgs,
		RunE:    runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	scope := managed.DetectScope()
	account, err := managed.InvokingAccount(scope)
	if err != nil {
		return err
	}
	if err := resolveManagedConfigRoot(cmd, scope, account); err != nil {
		return err
	}
	source, err := config.Source()
	if err != nil {
		return err
	}
	format, err := commandLogFormat(cmd)
	if err != nil {
		return err
	}
	verbose, debug := commandLogMode(cmd)
	if !source.Exists {
		if debug || format == logger.FormatJSON {
			log := commandLogger(cmd)
			log.Warning("STATUS", "status.not-initialized", "chatgpt-mcp is not initialized", nil)
			log.Detail("config", source.Path)
			return nil
		}
		renderStatusUninitialized(cmd.OutOrStdout())
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workspaces, err := workspace.NewManager(workspace.DefaultStorePath()).List()
	if err != nil {
		return err
	}
	upstreams := upstream.NewManager(upstream.NewStore(upstream.Path()))
	if err := upstreams.Load(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Second)
	runtimeStatus, running, runtimeErr := managedRuntimeStatus(ctx)
	cancel()
	if runtimeErr != nil {
		return runtimeErr
	}
	plan, listenerErr := resolveListenerPlan(cfg.Server.Expose)
	tunnelStatus := fetchTunnelStatus(cmd.Context(), cfg.Tunnel)
	snapshot := statusSnapshot{Source: source, Config: cfg, Runtime: runtimeStatus, Running: running, Workspaces: len(workspaces), Upstreams: len(upstreams.List()), ListenerPlan: plan, ListenerError: listenerErr, Tunnel: tunnelStatus}
	if !running {
		snapshot.Services = installedManagedServices(account)
	}
	if debug || format == logger.FormatJSON {
		renderLegacyStatus(cmd, snapshot)
		return nil
	}
	if snapshot.Running && transientTunnelState(statusTunnelState(snapshot.Runtime, true)) && logger.CanAnimate(cmd.OutOrStdout()) {
		renderStatusBaseText(cmd.OutOrStdout(), snapshot, verbose)
		fmt.Fprintln(cmd.OutOrStdout(), "\n"+cliHeading("Tunnel"))
		snapshot.Runtime = animateRuntimeTunnelState(cmd, snapshot.Runtime, statusTunnelWatchTimeout)
		snapshot.Tunnel.Running = snapshot.Runtime.TunnelRunning
		snapshot.Tunnel.Ready = snapshot.Runtime.TunnelReady
		snapshot.Tunnel.Restarting = snapshot.Runtime.TunnelRestarting
		snapshot.Tunnel.LastError = snapshot.Runtime.TunnelLastError
		renderStatusTunnelBody(cmd.OutOrStdout(), snapshot, verbose)
		return nil
	}
	renderStatusText(cmd.OutOrStdout(), snapshot, verbose)
	return nil
}

func renderStatusText(out io.Writer, snapshot statusSnapshot, verbose bool) {
	renderStatusBaseText(out, snapshot, verbose)
	renderStatusTunnel(out, snapshot, verbose)
}

func renderStatusBaseText(out io.Writer, snapshot statusSnapshot, verbose bool) {
	if snapshot.Running {
		fmt.Fprintln(out, cliStyled(color.FgHiGreen, color.Bold).Sprint("✓"), "ChatGPT MCP is running")
		renderRunningStatus(out, snapshot, verbose)
		return
	}
	fmt.Fprintln(out, cliStyled(color.FgHiRed, color.Bold).Sprint("×"), "ChatGPT MCP is stopped")
	renderStoppedStatus(out, snapshot, verbose)
}

func renderRunningStatus(out io.Writer, snapshot statusSnapshot, verbose bool) {
	status := snapshot.Runtime
	fmt.Fprintln(out, "\n"+cliHeading("Runtime"))
	statusField(out, "pid", status.PID)
	if status.RunID != "" {
		statusField(out, "session", shortSessionID(status.RunID))
	}
	if verbose && !status.StartedAt.IsZero() {
		statusField(out, "started", status.StartedAt.Local().Format(time.RFC3339))
	}
	if !status.StartedAt.IsZero() {
		statusField(out, "uptime", formatStatusUptime(status.StartedAt))
	}
	if verbose {
		statusField(out, "managed", status.Managed)
		if status.Managed {
			statusField(out, "scope", status.ServiceScope)
			statusField(out, "backend", runtimeBackendLabel(status.ServiceScope))
			statusField(out, "service", status.ServiceID)
		}
	} else if status.Managed {
		statusField(out, "managed", strings.TrimSpace(status.ServiceScope+" · "+runtimeBackendLabel(status.ServiceScope)))
		statusField(out, "service", status.ServiceID)
	} else {
		statusField(out, "mode", "foreground")
	}
	renderStatusEndpoints(out, snapshot, verbose)
	renderStatusConfig(out, snapshot, verbose)
}

func renderStoppedStatus(out io.Writer, snapshot statusSnapshot, verbose bool) {
	renderStatusEndpoints(out, snapshot, verbose)
	renderStatusConfig(out, snapshot, verbose)
	if len(snapshot.Services) == 0 {
		return
	}
	fmt.Fprintln(out, "\n"+cliHeading("Service"))
	for _, item := range snapshot.Services {
		statusField(out, string(item.spec.Scope), fmt.Sprintf("installed · %s", managedBackendLabel(item.manager, item.spec)))
	}
}

func renderStatusEndpoints(out io.Writer, snapshot statusSnapshot, verbose bool) {
	cfg := snapshot.Config
	fmt.Fprintln(out, "\n"+cliHeading("Endpoints"))
	if !verbose {
		statusField(out, "mcp", endpointURL(mcpnetwork.LoopbackHost, cfg.Server.Port, "/mcp"))
		if cfg.Admin.Enabled {
			statusField(out, "admin", endpointURL(mcpnetwork.LoopbackHost, cfg.Admin.Port, "/"))
		} else {
			statusField(out, "admin", "disabled")
		}
		statusField(out, "exposure", statusExposureSummary(snapshot))
		return
	}
	statusField(out, "expose", cfg.Server.Expose.Mode)
	if len(cfg.Server.Expose.Interfaces) > 0 {
		statusField(out, "interfaces", strings.Join(cfg.Server.Expose.Interfaces, ", "))
	}
	if snapshot.ListenerError != nil {
		statusField(out, "network", snapshot.ListenerError.Error())
		return
	}
	addresses := append([]mcpnetwork.Address(nil), snapshot.ListenerPlan.Addresses...)
	sort.SliceStable(addresses, func(i, j int) bool {
		left, right := statusAddressPriority(addresses[i]), statusAddressPriority(addresses[j])
		if left != right {
			return left < right
		}
		if addresses[i].Interface != addresses[j].Interface {
			return addresses[i].Interface < addresses[j].Interface
		}
		return addresses[i].Host < addresses[j].Host
	})
	for _, address := range addresses {
		name := address.Interface
		if name == "" {
			name = address.Scope
		}
		fmt.Fprintf(out, "\n  %s\n", cliHeading(name))
		statusNestedField(out, "mcp", endpointURL(address.Host, cfg.Server.Port, "/mcp"))
		if cfg.Admin.Enabled {
			statusNestedField(out, "admin", endpointURL(address.Host, cfg.Admin.Port, "/"))
		}
	}
	if !cfg.Admin.Enabled && len(addresses) == 0 {
		statusField(out, "admin", "disabled")
	}
}

func renderStatusTunnel(out io.Writer, snapshot statusSnapshot, verbose bool) {
	fmt.Fprintln(out, "\n"+cliHeading("Tunnel"))
	renderStatusTunnelBody(out, snapshot, verbose)
}

func renderStatusTunnelBody(out io.Writer, snapshot statusSnapshot, verbose bool) {
	status := snapshot.Runtime
	if !snapshot.Running {
		status = runtimeStatusResult{TunnelEnabled: snapshot.Config.Tunnel.Enabled, TunnelConfigured: tunnel.Configured(snapshot.Config.Tunnel), TunnelID: snapshot.Config.Tunnel.ID}
	}
	state := statusTunnelState(status, snapshot.Running)
	renderTunnelStateLine(out, state)
	if verbose {
		statusField(out, "enabled", status.TunnelEnabled)
		statusField(out, "configured", status.TunnelConfigured)
	}
	if status.TunnelID != "" {
		statusField(out, "id", status.TunnelID)
	}
	if snapshot.Tunnel.Metadata != nil {
		metadata := snapshot.Tunnel.Metadata
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
		}
	}
	if verbose && snapshot.Tunnel.AdminKeyConfigured && snapshot.Tunnel.AdminScope != nil {
		statusField(out, "admin", "configured · "+formatTunnelAdminScope(*snapshot.Tunnel.AdminScope))
	}
	if verbose && snapshot.Tunnel.MetadataError != "" {
		statusField(out, "metadata", "unavailable: "+snapshot.Tunnel.MetadataError)
	}
	if verbose && status.TunnelLastError != "" {
		statusField(out, "error", status.TunnelLastError)
	}
}

func renderStatusConfig(out io.Writer, snapshot statusSnapshot, verbose bool) {
	cfg := snapshot.Config
	fmt.Fprintln(out, "\n"+cliHeading("Config"))
	path := compactStatusPath(snapshot.Source.Path)
	if verbose {
		path = snapshot.Source.Path
		statusField(out, "initialized", snapshot.Source.Exists)
	}
	statusField(out, "file", path)
	if verbose {
		statusField(out, "format", snapshot.Source.Format)
	}
	statusField(out, "auth", fmt.Sprintf("mcp %s · admin %s", onOff(cfg.Auth.MCPEnabled), onOff(cfg.Auth.AdminEnabled)))
	statusField(out, "workspaces", snapshot.Workspaces)
	statusField(out, "upstreams", snapshot.Upstreams)
}

func renderStatusUninitialized(out io.Writer) {
	fmt.Fprintln(out, cliStyled(color.FgHiYellow, color.Bold).Sprint("!"), "ChatGPT MCP is not initialized")
	fmt.Fprintln(out, "\n"+cliHeading("Run:"))
	fmt.Fprintf(out, "  %s init\n", cliUseName())
}

func renderLegacyStatus(cmd *cobra.Command, snapshot statusSnapshot) {
	log := commandLogger(cmd)
	cfg, runtimeStatus := snapshot.Config, snapshot.Runtime
	log.Info("STATUS", "local runtime configuration")
	log.Detail("initialized", snapshot.Source.Exists)
	log.Detail("config", snapshot.Source.Path)
	log.Detail("format", snapshot.Source.Format)
	logEndpointDetails(log, cfg)
	log.Detail("auth", fmt.Sprintf("mcp=%t admin=%t", cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled))
	if snapshot.Running {
		log.Detail("runtime", "running")
		log.Detail("managed", runtimeStatus.Managed)
		if runtimeStatus.RunID != "" {
			log.Detail("session", shortSessionID(runtimeStatus.RunID))
		}
		log.Detail("pid", runtimeStatus.PID)
		if !runtimeStatus.StartedAt.IsZero() {
			log.Detail("started", runtimeStatus.StartedAt.Local().Format(time.RFC3339))
		}
		if runtimeStatus.Managed {
			log.Detail("scope", runtimeStatus.ServiceScope)
			log.Detail("backend", runtimeBackendLabel(runtimeStatus.ServiceScope))
			log.Detail("service", runtimeStatus.ServiceID)
		}
		log.Detail("tunnel", runtimeTunnelSummary(runtimeStatus))
		if runtimeStatus.TunnelID != "" {
			log.Detail("tunnel id", runtimeStatus.TunnelID)
		}
	} else {
		log.Detail("runtime", "stopped")
		state := runtimeStatusResult{TunnelEnabled: cfg.Tunnel.Enabled, TunnelConfigured: tunnel.Configured(cfg.Tunnel), TunnelID: cfg.Tunnel.ID}
		log.Detail("tunnel", runtimeTunnelSummary(state))
		if cfg.Tunnel.ID != "" {
			log.Detail("tunnel id", cfg.Tunnel.ID)
		}
		for _, item := range snapshot.Services {
			log.Detail("service "+string(item.spec.Scope), fmt.Sprintf("installed (%s)", managedBackendLabel(item.manager, item.spec)))
		}
	}
	if snapshot.Tunnel.Metadata != nil {
		log.Detail("tunnel name", snapshot.Tunnel.Metadata.Name)
		log.Detail("tunnel description", snapshot.Tunnel.Metadata.Description)
		if len(snapshot.Tunnel.Metadata.WorkspaceIDs) > 0 {
			log.Detail("tunnel workspaces", strings.Join(snapshot.Tunnel.Metadata.WorkspaceIDs, ", "))
		}
		if len(snapshot.Tunnel.Metadata.OrganizationIDs) > 0 {
			log.Detail("tunnel organizations", strings.Join(snapshot.Tunnel.Metadata.OrganizationIDs, ", "))
		}
	}
	if snapshot.Tunnel.MetadataError != "" {
		log.Detail("tunnel metadata error", snapshot.Tunnel.MetadataError)
	}
	log.Detail("workspaces", snapshot.Workspaces)
	log.Detail("upstreams", snapshot.Upstreams)
}

func statusField(out io.Writer, label string, value any) {
	fmt.Fprintf(out, "  %s %v\n", cliDim(fmt.Sprintf("%-11s", label)), value)
}
func statusNestedField(out io.Writer, label string, value any) {
	fmt.Fprintf(out, "    %s %v\n", cliDim(fmt.Sprintf("%-9s", label)), value)
}

func statusStateField(out io.Writer, label string, value any) {
	fmt.Fprintf(out, "  %s %s\n", cliDim(fmt.Sprintf("%-11s", label)), cliState(value))
}

func statusExposureSummary(snapshot statusSnapshot) string {
	mode := string(snapshot.Config.Server.Expose.Mode)
	if snapshot.ListenerError != nil {
		return mode + " · network unavailable"
	}
	count := statusNetworkInterfaceCount(snapshot.ListenerPlan.Addresses)
	if count == 0 {
		return mode
	}
	label := "network interfaces"
	if count == 1 {
		label = "network interface"
	}
	return fmt.Sprintf("%s · %d %s", mode, count, label)
}

func statusNetworkInterfaceCount(addresses []mcpnetwork.Address) int {
	seen := map[string]struct{}{}
	for _, address := range addresses {
		if address.Interface != "" {
			seen[address.Interface] = struct{}{}
		}
	}
	return len(seen)
}

func statusAddressPriority(address mcpnetwork.Address) int {
	if address.Interface == "" {
		return 0
	}
	name := strings.ToLower(address.Interface)
	switch {
	case strings.HasPrefix(name, "br-"), strings.HasPrefix(name, "veth"), strings.HasPrefix(name, "virbr"), strings.HasPrefix(name, "docker_gwbridge"):
		return 3
	case name == "docker0", strings.HasPrefix(name, "podman"):
		return 2
	default:
		return 1
	}
}

func statusTunnelState(status runtimeStatusResult, runtimeRunning bool) string {
	if !status.TunnelEnabled {
		return "disabled"
	}
	if !status.TunnelConfigured {
		return "not configured"
	}
	if !runtimeRunning {
		return "offline"
	}
	switch {
	case status.TunnelReady:
		return "connected"
	case status.TunnelRestarting:
		return "reconnecting"
	case status.TunnelRunning:
		return "connecting"
	case status.TunnelLastError != "":
		return "failed"
	default:
		return "starting"
	}
}

func transientTunnelState(state string) bool {
	return state == "starting" || state == "connecting" || state == "reconnecting"
}

func animateRuntimeTunnelState(cmd *cobra.Command, status runtimeStatusResult, timeout time.Duration) runtimeStatusResult {
	log := commandLogger(cmd)
	defer log.Close()
	state := statusTunnelState(status, true)
	log.Action("TUNNEL", "tunnel.status."+state, tunnelStateActionMessage(state))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Second)
		next, running, err := managedRuntimeStatus(ctx)
		cancel()
		if err != nil || !running {
			return status
		}
		status = next
		nextState := statusTunnelState(status, true)
		if !transientTunnelState(nextState) {
			return status
		}
		if nextState != state {
			state = nextState
			log.Action("TUNNEL", "tunnel.status."+state, tunnelStateActionMessage(state))
		}
		select {
		case <-cmd.Context().Done():
			return status
		case <-time.After(150 * time.Millisecond):
		}
	}
	return status
}

func tunnelStateActionMessage(state string) string {
	switch state {
	case "starting":
		return "Starting OpenAI Secure MCP Tunnel"
	case "reconnecting":
		return "Reconnecting OpenAI Secure MCP Tunnel"
	default:
		return "Connecting OpenAI Secure MCP Tunnel"
	}
}

func renderTunnelStateLine(out io.Writer, state string) {
	message := "OpenAI Secure MCP Tunnel is " + state
	switch state {
	case "connected":
		fmt.Fprintln(out, cliStyled(color.FgHiGreen, color.Bold).Sprint("✓"), message)
	case "starting", "connecting", "reconnecting":
		fmt.Fprintln(out, cliStyled(color.FgHiCyan, color.Bold).Sprint("⠋"), message)
	case "failed":
		fmt.Fprintln(out, cliStyled(color.FgHiRed, color.Bold).Sprint("×"), message)
	case "degraded":
		fmt.Fprintln(out, cliStyled(color.FgHiYellow, color.Bold).Sprint("!"), message)
	default:
		fmt.Fprintln(out, cliDim("·"), message)
	}
}

func formatStatusUptime(started time.Time) string {
	duration := time.Since(started).Round(time.Second)
	if duration < 0 {
		duration = 0
	}
	days := duration / (24 * time.Hour)
	duration %= 24 * time.Hour
	hours := duration / time.Hour
	duration %= time.Hour
	minutes := duration / time.Minute
	seconds := duration % time.Minute / time.Second
	if days > 0 {
		return fmt.Sprintf("%dd %02dh %02dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func compactStatusPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(home, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	if relative == "." {
		return "~"
	}
	return "~" + string(filepath.Separator) + relative
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

type installedManagedService struct {
	spec    managed.Spec
	manager managed.Manager
}

func installedManagedServices(account managed.Account) []installedManagedService {
	manager := managed.NewManager()
	scopes := []managed.Scope{managed.ScopeUser}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		scopes = append(scopes, managed.ScopeSystem)
	}
	result := make([]installedManagedService, 0, len(scopes))
	for _, scope := range scopes {
		spec := managed.Spec{ID: managed.ID(config.RootPath(), scope), Scope: scope, ConfigRoot: config.RootPath(), Account: account}
		status, err := manager.Status(spec)
		if err == nil && status.Installed {
			result = append(result, installedManagedService{spec: spec, manager: manager})
		}
	}
	return result
}

func runtimeBackendLabel(scope string) string {
	if runtime.GOOS == "linux" {
		if scope == string(managed.ScopeUser) {
			return "systemd --user"
		}
		return "systemd"
	}
	if runtime.GOOS == "darwin" {
		if scope == string(managed.ScopeUser) {
			return "launchd LaunchAgent"
		}
		return "launchd LaunchDaemon"
	}
	if runtime.GOOS == "windows" {
		return "task-scheduler"
	}
	return "unknown"
}
