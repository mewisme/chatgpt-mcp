package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/notification"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/redact"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/state"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

var errDoctorFailed = errors.New("doctor checks failed")

func doctorCommand() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Diagnose the local ChatGPT MCP installation", Args: cobra.NoArgs, RunE: runDoctor}
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	logCommandStep(cmd, "DOCTOR", "doctor.running", "Running diagnostics")
	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	state := &doctorState{}
	report := runDoctorChecks(parentCtx, state.checks())
	if state.upstreams != nil {
		_ = state.upstreams.Shutdown(parentCtx)
	}
	format, err := commandLogFormat(cmd)
	if err != nil {
		return err
	}
	verbose, _ := commandLogMode(cmd)
	out := cmd.OutOrStdout()
	if format == logger.FormatJSON {
		if err := renderDoctorJSON(out, report); err != nil {
			return err
		}
	} else if err := renderDoctorReport(out, report, verbose); err != nil {
		return err
	}
	if report.Fail > 0 {
		return errDoctorFailed
	}
	return nil
}

type doctorState struct {
	cfg       config.Config
	store     *pluginpkg.Store
	lock      pluginpkg.LockFile
	upstreams *upstream.Manager
}

func (d *doctorState) checks() []doctorCheck {
	checks := []doctorCheck{
		{ID: "install.current", Label: "install", Section: "System", Run: d.checkInstall},
		{ID: "config.source", Label: "config source", Section: "System", Run: d.checkConfigSource},
		{ID: "config.integrity", Label: "config integrity", Section: "System", Requires: []string{"config.source"}, Run: d.checkConfigIntegrity},
		{ID: "config.validate", Label: "config validate", Section: "System", Requires: []string{"config.source"}, Run: d.checkConfigValidate},
		{ID: "config.security", Label: "config security", Section: "System", Requires: []string{"config.validate"}, Run: d.checkConfigSecurity},
		{ID: "storage.paths", Label: "storage", Section: "System", Requires: []string{"config.source"}, Run: d.checkStoragePaths},
		{ID: "observability.events", Label: "runtime journal", Section: "System", Requires: []string{"storage.paths"}, Run: d.checkRuntimeJournal},
		{ID: "plugin.lock", Label: "plugin lock", Section: "Plugins", Run: d.checkPluginLock},
		{ID: "plugin.payloads", Label: "plugin payloads", Section: "Plugins", Requires: []string{"plugin.lock"}, Run: d.checkPluginPayloads},
		{ID: "plugin.desired", Label: "plugin desired", Section: "Plugins", Requires: []string{"plugin.lock"}, Run: d.checkPluginDesired},
		{ID: "plugin.compatibility", Label: "plugin compatibility", Section: "Plugins", Requires: []string{"plugin.lock"}, Run: d.checkPluginCompatibility},
		{ID: "plugin.host", Label: "plugin host", Section: "Plugins", Requires: []string{"plugin.payloads"}, Timeout: 3 * time.Second, Run: d.checkPluginHost},
		{ID: "plugin.capabilities", Label: "plugin capabilities", Section: "Plugins", Requires: []string{"plugin.lock"}, Run: d.checkPluginCapabilities},
		{ID: "plugin.admin-ui", Label: "Admin UI plugin", Section: "Plugins", Requires: []string{"plugin.lock", "config.source"}, Run: d.checkPluginAdminUI},
		{ID: "plugin.secure-mcp-tunnel", Label: "Secure MCP Tunnel plugin", Section: "Plugins", Requires: []string{"plugin.lock", "config.source"}, Run: d.checkPluginSecureMCP},
		{ID: "plugin.registry", Label: "plugin registry", Section: "Plugins", Requires: []string{"plugin.lock"}, Timeout: 3 * time.Second, Run: d.checkPluginRegistry},
		{ID: "runtime.control", Label: "runtime control", Section: "Runtime", Requires: []string{"config.source"}, Run: d.checkRuntimeControl},
		{ID: "service.user", Label: "user service", Section: "Runtime", Requires: []string{"config.source"}, Run: func(ctx context.Context) doctorResult { return d.checkService(ctx, "user") }},
		{ID: "service.system", Label: "system service", Section: "Runtime", Requires: []string{"config.source"}, Run: func(ctx context.Context) doctorResult { return d.checkService(ctx, "system") }},
		{ID: "network.plan", Label: "listener plan", Section: "Runtime", Requires: []string{"config.validate"}, Run: d.checkNetworkPlan},
		{ID: "network.health", Label: "HTTP health", Section: "Runtime", Requires: []string{"network.plan", "runtime.control"}, Timeout: 3 * time.Second, Run: d.checkNetworkHealth},
		{ID: "auth.mcp", Label: "Direct MCP HTTP auth", Section: "Runtime", Requires: []string{"config.validate"}, Run: d.checkAuthMCP},
		{ID: "auth.admin", Label: "Admin auth", Section: "Runtime", Requires: []string{"config.validate"}, Run: d.checkAuthAdmin},
		{ID: "shell.provider", Label: "Bash", Section: "Runtime", Requires: []string{"config.source"}, Run: d.checkShellProvider},
		{ID: "tools.registry", Label: "tool registry", Section: "Runtime", Requires: []string{"config.source"}, Run: d.checkToolsRegistry},
		{ID: "tunnel.collection", Label: "Secure MCP tunnels", Section: "Integrations", Requires: []string{"config.validate"}, Run: d.checkTunnelCollection},
		{ID: "cf-tunnel.mcp", Label: "CF Tunnel MCP", Section: "Integrations", Requires: []string{"config.source"}, Run: d.checkCFTunnelMCP},
		{ID: "cf-tunnel.admin", Label: "CF Tunnel Admin", Section: "Integrations", Requires: []string{"config.source"}, Run: d.checkCFTunnelAdmin},
		{ID: "notification.provider", Label: "notifications", Section: "Integrations", Requires: []string{"config.source"}, Timeout: time.Second, Run: d.checkNotificationProvider},
		{ID: "update.metadata", Label: "updates", Section: "Integrations", Timeout: 5 * time.Second, Run: d.checkUpdate},
	}
	return append(append(checks, d.workspaceChecks()...), d.upstreamChecks()...)
}

func (d *doctorState) checkConfigSource(ctx context.Context) doctorResult {
	source, err := config.Source()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "configuration source unavailable", Error: redact.Text(err.Error())}
	}
	if !source.Exists {
		return doctorResult{Status: doctorFail, Summary: "chatgpt-mcp is not initialized; run chatgpt-mcp init", Details: []string{source.Path}}
	}
	cfg, err := config.Load()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "configuration could not be loaded", Error: redact.Text(err.Error()), Details: []string{source.Path}}
	}
	d.cfg = cfg
	return doctorResult{Status: doctorPass, Summary: "configuration loaded", Details: []string{source.Path}}
}

func (d *doctorState) checkPluginLock(ctx context.Context) doctorResult {
	store, err := pluginpkg.NewStore(pluginpkg.DefaultLayout(), pluginpkg.RuntimeContext{})
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "plugin store unavailable", Error: redact.Text(err.Error())}
	}
	d.store = store
	lock, err := pluginpkg.LoadLock(store.Layout().LockPath())
	if err != nil {
		hint := "back up plugins.lock.json, then run cgm plugin verify"
		if errors.Is(err, pluginpkg.ErrLockCorrupt) {
			hint = "doctor does not quarantine the lock; fix or replace plugins.lock.json, then run cgm plugin verify"
		}
		return doctorResult{Status: doctorFail, Summary: "plugin lock could not be read", Error: redact.Text(err.Error()), Hint: hint}
	}
	d.lock = lock
	details := []string{}
	if d.store != nil && len(d.store.Builtins) > 0 {
		details = append(details, fmt.Sprintf("%d compiled builtins", len(d.store.Builtins)))
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("plugin lock decoded (%d entries)", len(lock.Plugins)), Details: details}
}

func (d *doctorState) checkPluginDesired(ctx context.Context) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	manager := &pluginpkg.Manager{Store: d.store, RegistryClient: pluginpkg.RegistryClient{Layout: pluginpkg.DefaultLayout(), UserAgent: "chatgpt-mcp/" + version.Version}}
	desired, err := manager.AssessDesired()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "plugin desired state unavailable", Error: redact.Text(err.Error())}
	}
	details := []string{fmt.Sprintf("%d desired, %d satisfied", desired.Desired, desired.Satisfied)}
	if len(desired.Missing) > 0 {
		details = append(details, "missing "+pluginIDsCSV(desired.Missing))
	}
	if len(desired.Incompatible) > 0 {
		details = append(details, "incompatible "+pluginIDsCSV(desired.Incompatible))
	}
	if len(desired.Pending) > 0 {
		details = append(details, "pending "+pluginIDsCSV(desired.Pending))
	}
	if desired.LockError != "" {
		details = append(details, redact.Text(desired.LockError))
	}
	if desired.Desired > desired.Satisfied {
		return doctorResult{Status: doctorWarn, Summary: "plugin desired state is not fully satisfied", Details: details}
	}
	return doctorResult{Status: doctorPass, Summary: "plugin desired state satisfied", Details: details}
}

func (d *doctorState) checkPluginCapabilities(ctx context.Context) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	conflicts, err := pluginpkg.CapabilityConflicts(d.store)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "plugin capability conflicts unavailable", Error: redact.Text(err.Error())}
	}
	if len(conflicts) == 0 {
		return doctorResult{Status: doctorPass, Summary: "no duplicate enabled capability providers"}
	}
	details := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		details = append(details, redact.Text(conflict.Error()))
	}
	return doctorResult{Status: doctorWarn, Summary: "duplicate enabled plugin capability providers", Details: details}
}

func (d *doctorState) checkPluginRegistry(ctx context.Context) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	manager := &pluginpkg.Manager{Store: d.store, RegistryClient: pluginpkg.RegistryClient{Layout: pluginpkg.DefaultLayout(), UserAgent: "chatgpt-mcp/" + version.Version}}
	health, err := manager.RegistryHealth(ctx)
	if err != nil {
		return doctorResult{Status: doctorWarn, Summary: "plugin registry diagnostics unavailable", Error: redact.Text(err.Error())}
	}
	details := make([]string, 0, len(health))
	cached, unavailable := false, false
	for _, item := range health {
		switch item.Status {
		case "verified":
			details = append(details, item.Registry.Name+" signed metadata verified")
		case "verified-cache":
			cached = true
			details = append(details, item.Registry.Name+" "+redact.Text(item.Error))
		default:
			unavailable = true
			details = append(details, item.Registry.Name+" "+redact.Text(item.Error))
		}
	}
	if unavailable {
		return doctorResult{Status: doctorWarn, Summary: "plugin registry signature/metadata unavailable", Details: details}
	}
	if cached {
		return doctorResult{Status: doctorWarn, Summary: "plugin registry using verified cached metadata", Details: details}
	}
	return doctorResult{Status: doctorPass, Summary: "plugin registry metadata verified", Details: details}
}

func (d *doctorState) checkShellProvider(ctx context.Context) doctorResult {
	resolver := shellruntime.NewProviderResolver(d.store)
	if err := resolver.SetConfiguredExecutable(d.cfg.Shell.Executable); err != nil {
		return doctorResult{Status: doctorFail, Summary: "Bash shell provider configuration invalid", Error: redact.Text(err.Error())}
	}
	provider, err := resolver.Resolve()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "Bash shell provider unavailable", Error: redact.Text(err.Error())}
	}
	details := []string{provider.Label(), provider.Executable}
	if provider.Version != "" {
		details = append(details, string(provider.Version))
	}
	return doctorResult{Status: doctorPass, Summary: "Bash shell provider ready", Details: details}
}

func (d *doctorState) checkNotificationProvider(ctx context.Context) doctorResult {
	if err := notification.ValidateSettings(d.cfg.Notifications); err != nil {
		return doctorResult{Status: doctorFail, Summary: "notification settings are invalid", Error: redact.Text(err.Error())}
	}
	provider := notification.PlatformProvider()
	available := provider.Available(ctx)
	if !d.cfg.Notifications.Enabled {
		summary := "desktop notifications disabled"
		if !available {
			summary = "desktop notifications disabled · provider unavailable"
		}
		return doctorResult{Status: doctorSkip, Summary: summary}
	}
	if !available {
		return doctorResult{Status: doctorWarn, Summary: "desktop notifications enabled but provider unavailable", Hint: "install a desktop notifier or disable notifications.enabled"}
	}
	caps := provider.Capabilities(ctx)
	details := []string{}
	if caps.Notification {
		details = append(details, "notification")
	}
	if caps.Actions {
		details = append(details, "actions")
	}
	if caps.OpenTerminal {
		details = append(details, "open-terminal")
	}
	if d.cfg.Notifications.OpenAction == notification.OpenActionAuto && !caps.Actions {
		return doctorResult{Status: doctorWarn, Summary: "open_action=auto but the host cannot offer notification actions", Details: details}
	}
	active, err := state.TUIReviewerActive()
	if err != nil {
		details = append(details, "TUI presence "+redact.Text(err.Error()))
	} else if active {
		details = append(details, "TUI reviewer present")
	}
	return doctorResult{Status: doctorPass, Summary: "desktop notification provider available", Details: details}
}

func pluginIDsCSV(ids []pluginpkg.PluginID) string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}
	return strings.Join(values, ", ")
}
