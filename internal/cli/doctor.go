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
	cfg   config.Config
	store *pluginpkg.Store
}

func (d *doctorState) checks() []doctorCheck {
	return []doctorCheck{
		{ID: "install.current", Section: "System", Run: d.checkInstall},
		{ID: "config.source", Section: "System", Run: d.checkConfigSource},
		{ID: "config.integrity", Section: "System", Requires: []string{"config.source"}, Run: d.checkConfigIntegrity},
		{ID: "config.validate", Section: "System", Requires: []string{"config.source"}, Run: d.checkConfigValidate},
		{ID: "config.security", Section: "System", Requires: []string{"config.validate"}, Run: d.checkConfigSecurity},
		{ID: "storage.paths", Section: "System", Requires: []string{"config.source"}, Run: d.checkStoragePaths},
		{ID: "plugin.lock", Section: "Plugins", Run: d.checkPluginLock},
		{ID: "plugin.desired", Section: "Plugins", Requires: []string{"plugin.lock"}, Run: d.checkPluginDesired},
		{ID: "plugin.capabilities", Section: "Plugins", Requires: []string{"plugin.lock"}, Run: d.checkPluginCapabilities},
		{ID: "plugin.registry", Section: "Plugins", Requires: []string{"plugin.lock"}, Timeout: 3 * time.Second, Run: d.checkPluginRegistry},
		{ID: "runtime.control", Section: "Runtime", Requires: []string{"config.source"}, Run: d.checkRuntimeControl},
		{ID: "network.plan", Section: "Runtime", Requires: []string{"config.validate"}, Run: d.checkNetworkPlan},
		{ID: "auth.mcp", Section: "Runtime", Requires: []string{"config.validate"}, Run: d.checkAuthMCP},
		{ID: "auth.admin", Section: "Runtime", Requires: []string{"config.validate"}, Run: d.checkAuthAdmin},
		{ID: "shell.provider", Section: "Runtime", Requires: []string{"config.source"}, Run: d.checkShellProvider},
		{ID: "notification.provider", Section: "Integrations", Requires: []string{"config.source"}, Timeout: time.Second, Run: d.checkNotificationProvider},
	}
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
	reconcile, err := pluginpkg.Reconcile(store)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "plugin lock could not be inspected", Error: redact.Text(err.Error())}
	}
	details := []string{}
	if reconcile.QuarantinePath != "" {
		details = append(details, "quarantine "+reconcile.QuarantinePath)
	}
	if len(reconcile.Disabled) > 0 {
		ids := make([]string, len(reconcile.Disabled))
		for index, id := range reconcile.Disabled {
			ids[index] = string(id)
		}
		details = append(details, "disabled "+strings.Join(ids, ", "))
		for _, id := range reconcile.Disabled {
			if reason := reconcile.Issues[id]; reason != "" {
				details = append(details, string(id)+" "+redact.Text(reason))
			}
		}
		return doctorResult{Status: doctorWarn, Summary: "unsafe plugin activation state disabled", Details: details}
	}
	if reconcile.CorruptLock {
		return doctorResult{Status: doctorWarn, Summary: "corrupt plugin lock quarantined; plugins require explicit repair", Details: details}
	}
	return doctorResult{Status: doctorPass, Summary: "plugin lock and active payload metadata verified", Details: details}
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
	available := notification.PlatformProvider().Available(ctx)
	if !d.cfg.Notifications.Enabled {
		summary := "desktop notifications disabled"
		if !available {
			summary = "desktop notifications disabled · provider unavailable"
		}
		return doctorResult{Status: doctorSkip, Summary: summary}
	}
	if !available {
		return doctorResult{Status: doctorWarn, Summary: "desktop notifications enabled but provider unavailable"}
	}
	return doctorResult{Status: doctorPass, Summary: "desktop notification provider available"}
}

func pluginIDsCSV(ids []pluginpkg.PluginID) string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}
	return strings.Join(values, ", ")
}
