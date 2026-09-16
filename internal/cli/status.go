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
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
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
	Update        *updatepkg.CachedCheck
	TunnelNames   map[string]string
}

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "Show runtime health and local configuration",
		Args:    cobra.NoArgs,
		RunE:    runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) (runErr error) {
	ctx := cmd.Context()
	snapshotSpan := tracepkg.Start(ctx, "STATUS", "status.snapshot", "Acquiring status snapshot")
	snapshotComplete := false
	defer func() {
		if snapshotComplete {
			return
		}
		if runErr != nil {
			snapshotSpan.FailMessage("Status snapshot acquisition failed", runErr)
		} else {
			snapshotSpan.EndMessage("Status snapshot acquisition completed")
		}
	}()
	logCommandStep(cmd, "STATUS", "status.scope.resolving", "Resolving service scope")
	serviceSpan := tracepkg.Start(ctx, "STATUS", "status.service-context", "Resolving status service context")
	scope := managed.DetectScope()
	account, err := managed.InvokingAccountContext(ctx, scope)
	if err != nil {
		serviceSpan.FailMessage("Status service account resolution failed", err, tracepkg.String("scope", string(scope)))
		return err
	}
	if err := resolveManagedConfigRoot(cmd, scope, account); err != nil {
		serviceSpan.FailMessage("Status configuration root resolution failed", err, tracepkg.String("scope", string(scope)), tracepkg.String("account", account.Username))
		return err
	}
	serviceSpan.EndMessage("Status service context resolved", tracepkg.String("scope", string(scope)), tracepkg.String("account", account.Username), tracepkg.String("config_root", config.RootPath()))
	configSpan := tracepkg.Start(ctx, "STATUS", "status.config.load", "Loading status configuration", tracepkg.String("config_root", config.RootPath()))
	source, err := config.Source()
	if err != nil {
		configSpan.FailMessage("Status config source discovery failed", err)
		return err
	}
	format, err := commandLogFormat(cmd)
	if err != nil {
		configSpan.FailMessage("Status output format resolution failed", err, tracepkg.String("path", source.Path))
		return err
	}
	verbose, debug := commandLogMode(cmd)
	if !source.Exists {
		configSpan.EndMessage("Status configuration is not initialized", tracepkg.String("path", source.Path), tracepkg.String("format", string(source.Format)), tracepkg.Bool("exists", false))
		snapshotSpan.EndMessage("Status snapshot acquired", tracepkg.Bool("initialized", false), tracepkg.Bool("running", false))
		snapshotComplete = true
		if debug || format == logger.FormatJSON {
			log := commandLogger(cmd)
			log.Warning("STATUS", "status.not-initialized", "chatgpt-mcp is not initialized", nil)
			log.Detail("config", source.Path)
			return nil
		}
		renderStatusUninitialized(cmd.OutOrStdout())
		return nil
	}
	logCommandStep(cmd, "STATUS", "status.config.loading", "Loading runtime configuration")
	cfg, err := config.Load()
	if err != nil {
		configSpan.FailMessage("Status configuration load failed", err, tracepkg.String("path", source.Path), tracepkg.String("format", string(source.Format)))
		return err
	}
	configSpan.EndMessage("Status configuration loaded", tracepkg.String("path", source.Path), tracepkg.String("format", string(source.Format)), tracepkg.Bool("exists", true))
	workspaceSpan := tracepkg.Start(ctx, "STATUS", "status.workspaces.query", "Querying workspace count")
	workspaces, err := workspaceManagerForCommand(cmd).List()
	if err != nil {
		workspaceSpan.FailMessage("Workspace count query failed", err)
		return err
	}
	workspaceSpan.EndMessage("Workspace count queried", tracepkg.Int("count", len(workspaces)))
	upstreamSpan := tracepkg.Start(ctx, "STATUS", "status.upstreams.query", "Querying upstream MCP count")
	upstreams, err := loadUpstreamManagerForCommand(cmd)
	if err != nil {
		upstreamSpan.FailMessage("Upstream MCP count query failed", err)
		return err
	}
	upstreamCount := len(upstreams.List())
	upstreamSpan.EndMessage("Upstream MCP count queried", tracepkg.Int("count", upstreamCount))
	logCommandStep(cmd, "STATUS", "status.runtime.inspecting", "Inspecting runtime control endpoint")
	runtimeSpan := tracepkg.Start(ctx, "STATUS", "status.runtime-control.query", "Querying runtime control status")
	runtimeCtx, cancel := context.WithTimeout(ctx, time.Second)
	runtimeStatus, running, runtimeErr := managedRuntimeStatus(runtimeCtx)
	cancel()
	if runtimeErr != nil {
		runtimeSpan.FailMessage("Runtime control status query failed", runtimeErr)
		return runtimeErr
	}
	runtimeSpan.EndMessage("Runtime control status queried", tracepkg.Bool("running", running), tracepkg.Int("pid", runtimeStatus.PID), tracepkg.String("lifecycle", runtimeStatus.Lifecycle))
	listenerSpan := tracepkg.Start(ctx, "STATUS", "status.listener-plan.resolve", "Resolving status listener plan", tracepkg.String("exposure_mode", string(cfg.Server.Expose.Mode)), tracepkg.Any("interfaces", append([]string(nil), cfg.Server.Expose.Interfaces...)))
	plan, listenerErr := resolveListenerPlan(cfg.Server.Expose)
	if listenerErr != nil {
		listenerSpan.FailMessage("Status listener plan resolution failed", listenerErr)
	} else {
		listenerSpan.EndMessage("Status listener plan resolved", tracepkg.Int("host_count", len(plan.Hosts)), tracepkg.Int("address_count", len(plan.Addresses)))
	}
	updateSpan := tracepkg.Start(ctx, "STATUS", "status.update-cache.lookup", "Looking up cached update status")
	cachedUpdate := cachedUpdateStatus(time.Now())
	updateSpan.EndMessage("Cached update status lookup completed", tracepkg.Bool("cached", cachedUpdate != nil))
	snapshot := statusSnapshot{Source: source, Config: cfg, Runtime: runtimeStatus, Running: running, Workspaces: len(workspaces), Upstreams: upstreamCount, ListenerPlan: plan, ListenerError: listenerErr, Update: cachedUpdate, TunnelNames: loadCachedTunnelNames(cfg.RuntimeTunnels())}
	if !running {
		serviceInspectSpan := tracepkg.Start(ctx, "STATUS", "status.managed-services.inspect", "Inspecting installed managed services")
		snapshot.Services = installedManagedServices(ctx, account)
		serviceInspectSpan.EndMessage("Installed managed services inspected", tracepkg.Int("count", len(snapshot.Services)))
	}
	snapshotSpan.EndMessage("Status snapshot acquired", tracepkg.Bool("initialized", true), tracepkg.Bool("running", running), tracepkg.Int("workspaces", snapshot.Workspaces), tracepkg.Int("upstreams", snapshot.Upstreams), tracepkg.Int("managed_services", len(snapshot.Services)), tracepkg.Bool("listener_plan_available", listenerErr == nil), tracepkg.Bool("update_cached", cachedUpdate != nil))
	snapshotComplete = true
	if debug || format == logger.FormatJSON {
		renderLegacyStatus(cmd, snapshot)
		return nil
	}
	renderStatusText(cmd.OutOrStdout(), snapshot, verbose)
	return nil
}

func renderStatusText(out io.Writer, snapshot statusSnapshot, verbose bool) {
	renderStatusBaseText(out, snapshot, verbose)
	renderStatusTunnel(out, snapshot, verbose)
	renderStatusCFTunnel(out, snapshot)
}

func renderStatusBaseText(out io.Writer, snapshot statusSnapshot, verbose bool) {
	if snapshot.Running {
		if snapshot.Runtime.Starting {
			fmt.Fprintln(out, cliStyled(color.FgHiYellow, color.Bold).Sprint("·"), "ChatGPT MCP is starting")
		} else {
			fmt.Fprintln(out, cliStyled(color.FgHiGreen, color.Bold).Sprint("✓"), "ChatGPT MCP is running")
		}
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
		if cfg.Server.Enabled {
			statusField(out, "mcp http", endpointURL(mcpnetwork.LoopbackHost, cfg.Server.Port, "/mcp"))
		} else {
			statusField(out, "mcp http", "disabled")
		}
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
		if cfg.Server.Enabled {
			statusNestedField(out, "mcp http", endpointURL(address.Host, cfg.Server.Port, "/mcp"))
		}
		if cfg.Admin.Enabled {
			statusNestedField(out, "admin", endpointURL(address.Host, cfg.Admin.Port, "/"))
		}
	}
	if !cfg.Server.Enabled {
		statusField(out, "mcp http", "disabled")
	}
	if !cfg.Admin.Enabled && len(addresses) == 0 {
		statusField(out, "admin", "disabled")
	}
}

func renderStatusTunnel(out io.Writer, snapshot statusSnapshot, verbose bool) {
	fmt.Fprintln(out, "\n"+cliHeading("Tunnels"))
	renderStatusTunnelBody(out, snapshot, verbose)
}

func renderStatusTunnelBody(out io.Writer, snapshot statusSnapshot, verbose bool) {
	summary, items := statusTunnelCollection(snapshot)
	views := buildStatusTunnelViewsFromItems(items, snapshot.Running, snapshot.TunnelNames)
	if verbose {
		statusField(out, "total", summary.Total)
		statusField(out, "enabled", summary.Enabled)
		statusField(out, "configured", summary.Configured)
		statusField(out, "running", summary.Running)
		statusField(out, "ready", summary.Ready)
		statusField(out, "restarting", summary.Restarting)
		statusField(out, "degraded", summary.Degraded)
		for _, view := range views {
			fmt.Fprintf(out, "\n  %s\n", cliHeading(view.Label))
			fmt.Fprintf(out, "    %s %s\n", cliDim(fmt.Sprintf("%-9s", "state")), cliState(view.State))
			statusNestedField(out, "id", view.ID)
			if view.LastError != "" {
				statusNestedField(out, "error", view.LastError)
			}
		}
		return
	}
	statusField(out, "status", statusTunnelSummaryLine(snapshot.Running, summary, views))
	for _, view := range views {
		statusStateField(out, view.Label, view.State)
	}
}

func renderStatusCFTunnel(out io.Writer, snapshot statusSnapshot) {
	var live *runtimecontrol.CFTunnelStatus
	if snapshot.Running {
		live = snapshot.Runtime.CFTunnel
	}
	items := cfTunnelStatusItems(snapshot.Config, live)
	interesting := live != nil && live.PluginEnabled
	for _, item := range items {
		if item.Desired || item.Running || item.Ready || item.Restarting || item.LastError != "" {
			interesting = true
		}
	}
	if !interesting {
		return
	}
	fmt.Fprintln(out, "\n"+cliHeading("CF Tunnel"))
	statusField(out, "note", "ephemeral Quick Tunnels, not Secure MCP")
	for _, item := range items {
		statusField(out, item.Target, cfTunnelStatusLine(item))
	}
}

func statusTunnelCollection(snapshot statusSnapshot) (runtimecontrol.TunnelSummary, []runtimecontrol.TunnelRuntimeStatus) {
	if snapshot.Running {
		items := runtimeStatusTunnelItems(snapshot.Runtime)
		if snapshot.Runtime.TunnelSummary.Total > 0 || len(items) > 0 {
			return snapshot.Runtime.TunnelSummary, append([]runtimecontrol.TunnelRuntimeStatus(nil), items...)
		}
		return runtimecontrol.TunnelSummary{}, nil
	}
	collection := snapshot.Config.RuntimeTunnels()
	summary := runtimecontrol.TunnelSummary{Total: len(collection.Instances)}
	items := make([]runtimecontrol.TunnelRuntimeStatus, 0, len(collection.Instances))
	for _, instance := range collection.Instances {
		configured := strings.TrimSpace(instance.APIKey) != ""
		item := runtimecontrol.TunnelRuntimeStatus{ID: instance.ID, Name: snapshot.TunnelNames[instance.ID], Enabled: instance.Enabled, Configured: configured}
		items = append(items, item)
		if instance.Enabled {
			summary.Enabled++
		}
		if configured {
			summary.Configured++
		}
	}
	return summary, items
}

func tunnelRuntimeState(item runtimecontrol.TunnelRuntimeStatus, runtimeRunning bool) string {
	switch {
	case !item.Enabled:
		return "disabled"
	case !item.Configured:
		return "not configured"
	case !runtimeRunning:
		return "offline"
	case item.Ready:
		return "connected"
	case item.Restarting:
		return "reconnecting"
	case item.Running:
		return "connecting"
	case item.LastError != "":
		return "degraded"
	default:
		return "starting"
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
	collection := cfg.RuntimeTunnels()
	enabledTunnels := 0
	for _, instance := range collection.Instances {
		if instance.Enabled {
			enabledTunnels++
		}
	}
	statusField(out, "transports", fmt.Sprintf("http %s · tunnels %d/%d enabled", onOff(cfg.Server.Enabled), enabledTunnels, len(collection.Instances)))
	statusField(out, "auth", fmt.Sprintf("mcp %s · admin %s", onOff(cfg.Auth.MCPEnabled), onOff(cfg.Auth.AdminEnabled)))
	for _, warning := range config.SecurityWarnings(cfg) {
		fmt.Fprintln(out, "  "+cliStyled(color.FgHiYellow, color.Bold).Sprint("!")+" "+warning)
	}
	statusField(out, "workspaces", snapshot.Workspaces)
	statusField(out, "upstreams", snapshot.Upstreams)
	if snapshot.Update != nil && (verbose || snapshot.Update.Status == updatepkg.StatusAvailable) {
		statusField(out, "update", formatCachedUpdate(snapshot.Update))
		if verbose {
			statusField(out, "checked", snapshot.Update.CheckedAt.Local().Format(time.RFC3339))
		}
	}
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
	collection := cfg.RuntimeTunnels()
	enabledTunnels := 0
	for _, instance := range collection.Instances {
		if instance.Enabled {
			enabledTunnels++
		}
	}
	log.Detail("transports", fmt.Sprintf("http=%t tunnels=%d/%d", cfg.Server.Enabled, enabledTunnels, len(collection.Instances)))
	logEndpointDetails(log, cfg)
	log.Detail("auth", fmt.Sprintf("mcp=%t admin=%t", cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled))
	for _, warning := range config.SecurityWarnings(cfg) {
		name := "status.security-warning"
		switch {
		case strings.Contains(warning, "unauthenticated loopback"):
			name = "status.unauthenticated-loopback"
		case strings.Contains(warning, "cleartext HTTP"):
			name = "status.cleartext-http"
		}
		log.Warning("STATUS", name, warning, nil)
	}
	if snapshot.Running {
		runtimeState := "running"
		if runtimeStatus.Starting {
			runtimeState = "starting"
		}
		log.Detail("runtime", runtimeState)
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
	} else {
		log.Detail("runtime", "stopped")
		for _, item := range snapshot.Services {
			log.Detail("service "+string(item.spec.Scope), fmt.Sprintf("installed (%s)", managedBackendLabel(item.manager, item.spec)))
		}
	}
	summary, items := statusTunnelCollection(snapshot)
	log.Detail("tunnels", fmt.Sprintf("%d attached · %d enabled · %d ready · %d degraded", summary.Total, summary.Enabled, summary.Ready, summary.Degraded))
	for _, item := range items {
		log.Detail("tunnel "+item.ID, tunnelRuntimeState(item, snapshot.Running))
	}
	log.Detail("workspaces", snapshot.Workspaces)
	log.Detail("upstreams", snapshot.Upstreams)
	logCachedUpdate(log, snapshot.Update)
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
	if items := runtimeStatusTunnelItems(status); len(items) > 0 {
		return tunnelRuntimeState(items[0], runtimeRunning)
	}
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
		return "degraded"
	default:
		return "starting"
	}
}

func transientTunnelState(state string) bool {
	return state == "starting" || state == "connecting" || state == "reconnecting"
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

func installedManagedServices(ctx context.Context, account managed.Account) []installedManagedService {
	manager := managed.NewManagerWithObserver(tracepkg.ObserverFromContext(ctx))
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
